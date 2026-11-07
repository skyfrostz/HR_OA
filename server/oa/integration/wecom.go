package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type WeComConfig struct {
	Mode                 string
	CorpID               string
	Secret               string
	AgentID              int64
	MockAPIBase          string
	MockAuthorizeURL     string
	DeliverNotifications bool
}

var corpPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,64}$`)
var memberPattern = regexp.MustCompile(`^[A-Za-z0-9_.@-]{1,64}$`)

func NormalizeMemberID(input string) (string, bool) {
	input = strings.TrimSpace(input)
	value := strings.ToLower(input)
	return value, value != "@all" && memberPattern.MatchString(input)
}

func (c WeComConfig) Enabled() bool { return c.Mode == "wecom" || c.Mode == "mock" }

func (c WeComConfig) Validate(origin string, production bool) error {
	switch c.Mode {
	case "", "disabled":
		if c.DeliverNotifications || c.MockAPIBase != "" || c.MockAuthorizeURL != "" {
			return errors.New("invalid OA_WECOM_MODE")
		}
		return nil
	case "mock":
		api, apiOK := endpoint(c.MockAPIBase, false)
		auth, authOK := endpoint(c.MockAuthorizeURL, true)
		if production || !apiOK || !authOK || !localHost(api.Hostname()) || !localHost(auth.Hostname()) {
			return errors.New("invalid WeCom mock endpoints")
		}
	case "wecom":
		if c.MockAPIBase != "" || c.MockAuthorizeURL != "" {
			return errors.New("invalid WeCom live endpoints")
		}
	default:
		return errors.New("invalid OA_WECOM_MODE")
	}
	if !ValidateOrigin(origin, production || c.Mode == "wecom") {
		return errors.New("invalid OA_PUBLIC_ORIGIN for WeCom")
	}
	if !corpPattern.MatchString(c.CorpID) || c.AgentID < 1 || c.AgentID > 2147483647 ||
		len(c.Secret) < 8 || len(c.Secret) > 256 || strings.ContainsAny(c.Secret, "\r\n\t ") {
		return errors.New("invalid WeCom application configuration")
	}
	return nil
}

type WeCom struct {
	config     WeComConfig
	origin     string
	apiBase    string
	authURL    string
	client     *http.Client
	mu         sync.Mutex
	token      string
	tokenUntil time.Time
}

func NewWeCom(config WeComConfig, origin string) *WeCom {
	if !config.Enabled() {
		return nil
	}
	client := &WeCom{
		config: config, origin: strings.TrimRight(origin, "/"),
		apiBase: "https://qyapi.weixin.qq.com", authURL: "https://open.weixin.qq.com/connect/oauth2/authorize",
		client: &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}
	if config.Mode == "mock" {
		client.apiBase, client.authURL = strings.TrimRight(config.MockAPIBase, "/"), config.MockAuthorizeURL
	}
	return client
}

func (w *WeCom) CorpID() string { return w.config.CorpID }
func (w *WeCom) Origin() string { return w.origin }
func (w *WeCom) Mode() string   { return w.config.Mode }
func (w *WeCom) DeliveryMode() string {
	if !w.config.DeliverNotifications {
		return "mock"
	}
	if w.config.Mode == "mock" {
		return "wecom-mock"
	}
	return "wecom"
}

func (w *WeCom) AuthorizationURL(state, entry string) string {
	query := url.Values{
		"appid": {w.config.CorpID}, "agentid": {strconv.FormatInt(w.config.AgentID, 10)},
		"redirect_uri":  {w.origin + "/api/oa/v1/auth/wecom/callback"},
		"response_type": {"code"}, "scope": {"snsapi_base"}, "state": {state},
	}
	if entry == "pc" {
		query.Del("response_type")
		query.Del("scope")
		query.Set("login_type", "CorpApp")
		base := "https://login.work.weixin.qq.com/wwlogin/sso/login"
		if w.config.Mode == "mock" {
			base = w.authURL
		}
		return base + "?" + query.Encode()
	}
	return w.authURL + "?" + query.Encode() + "#wechat_redirect"
}

type wecomResponse struct {
	Code           *int   `json:"errcode"`
	Token          string `json:"access_token"`
	ExpiresIn      int    `json:"expires_in"`
	MemberID       string `json:"userid"`
	InvalidUser    string `json:"invaliduser"`
	UnlicensedUser string `json:"unlicenseduser"`
}

type providerCode int

func (providerCode) Error() string { return "WeCom rejected request" }

var errWeComTransport = errors.New("WeCom request outcome unavailable")
var ErrNotMember = errors.New("WeCom identity is not a configured enterprise member")

func (w *WeCom) call(ctx context.Context, method, path string, query url.Values, body []byte) (wecomResponse, error) {
	var result wecomResponse
	var source io.Reader
	if body != nil {
		source = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, w.apiBase+path+"?"+query.Encode(), source)
	if err != nil {
		return result, errWeComTransport
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	reply, err := w.client.Do(req)
	if err != nil {
		return result, errWeComTransport
	}
	defer reply.Body.Close()
	if reply.StatusCode != http.StatusOK {
		return result, errWeComTransport
	}
	data, err := io.ReadAll(io.LimitReader(reply.Body, 65537))
	if err != nil || len(data) > 65536 || json.Unmarshal(data, &result) != nil || result.Code == nil {
		return result, errWeComTransport
	}
	if *result.Code != 0 {
		return result, providerCode(*result.Code)
	}
	return result, nil
}

func (w *WeCom) accessToken(ctx context.Context, stale string) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", errWeComTransport
	}
	if w.token != "" && w.token != stale && time.Now().Before(w.tokenUntil) {
		return w.token, nil
	}
	reply, err := w.call(ctx, http.MethodGet, "/cgi-bin/gettoken", url.Values{
		"corpid": {w.config.CorpID}, "corpsecret": {w.config.Secret},
	}, nil)
	if err != nil {
		return "", err
	}
	if reply.Token == "" || len(reply.Token) > 4096 || reply.ExpiresIn < 1 || reply.ExpiresIn > 86400 {
		return "", errWeComTransport
	}
	w.token = reply.Token
	w.tokenUntil = time.Now().Add(time.Duration(reply.ExpiresIn-min(60, reply.ExpiresIn/10)) * time.Second)
	return w.token, nil
}

func (w *WeCom) authenticated(ctx context.Context, method, path string, query url.Values, body []byte) (wecomResponse, bool, error) {
	stale := ""
	for attempt := 0; attempt < 2; attempt++ {
		token, err := w.accessToken(ctx, stale)
		if err != nil {
			return wecomResponse{}, false, err
		}
		query.Set("access_token", token)
		reply, err := w.call(ctx, method, path, query, body)
		if err == providerCode(40014) || err == providerCode(42001) {
			stale = token
			continue
		}
		return reply, true, err
	}
	return wecomResponse{}, false, providerCode(40014)
}

func (w *WeCom) Member(ctx context.Context, code string) (string, error) {
	if len(code) < 1 || len(code) > 512 || strings.ContainsAny(code, "\r\n\x00") {
		return "", ErrNotMember
	}
	reply, _, err := w.authenticated(ctx, http.MethodGet, "/cgi-bin/auth/getuserinfo", url.Values{"code": {code}}, nil)
	if err != nil {
		return "", err
	}
	member, ok := NormalizeMemberID(reply.MemberID)
	if !ok {
		return "", ErrNotMember
	}
	return member, nil
}

type DeliveryOutcome struct {
	Status string
	Retry  bool
}

func (w *WeCom) Send(ctx context.Context, member, title string, eventID uint) DeliveryOutcome {
	member, valid := NormalizeMemberID(member)
	if !valid {
		return DeliveryOutcome{Status: "FAILED"}
	}
	// All message text is generated from fixed OA event labels.
	payload := struct {
		ToUser            string            `json:"touser"`
		MsgType           string            `json:"msgtype"`
		AgentID           int64             `json:"agentid"`
		Text              map[string]string `json:"text"`
		DuplicateCheck    int               `json:"enable_duplicate_check"`
		DuplicateInterval int               `json:"duplicate_check_interval"`
	}{
		ToUser: member, MsgType: "text", AgentID: w.config.AgentID,
		Text:           map[string]string{"content": title + "。请进入 HR OA 查看。事件 " + strconv.FormatUint(uint64(eventID), 10)},
		DuplicateCheck: 1, DuplicateInterval: 1800,
	}
	body, _ := json.Marshal(payload)
	reply, sent, err := w.authenticated(ctx, http.MethodPost, "/cgi-bin/message/send", url.Values{}, body)
	switch {
	case err == nil && (reply.InvalidUser != "" || reply.UnlicensedUser != ""):
		return DeliveryOutcome{Status: "FAILED"}
	case err == nil:
		return DeliveryOutcome{Status: "ACCEPTED"}
	case err == providerCode(-1) || err == providerCode(45009) || (!sent && err == errWeComTransport):
		return DeliveryOutcome{Status: "FAILED", Retry: true}
	case sent && err == errWeComTransport:
		return DeliveryOutcome{Status: "UNCERTAIN"}
	default:
		return DeliveryOutcome{Status: "FAILED"}
	}
}
