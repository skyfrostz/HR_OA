package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
)

func TestWeComAuthorizationEntries(t *testing.T) {
	const origin = "https://oa.example.test"
	config := WeComConfig{Mode: "wecom", CorpID: "ww-example", Secret: "synthetic-secret", AgentID: 1000001}
	if err := config.Validate(origin, true); err != nil {
		t.Fatal(err)
	}
	client := NewWeCom(config, origin)
	for _, tc := range []struct {
		entry, host, path, scope, loginType, fragment string
	}{
		{"pc", "login.work.weixin.qq.com", "/wwlogin/sso/login", "", "CorpApp", ""},
		{"h5", "open.weixin.qq.com", "/connect/oauth2/authorize", "snsapi_base", "", "wechat_redirect"},
	} {
		t.Run(tc.entry, func(t *testing.T) {
			target, err := url.Parse(client.AuthorizationURL("sampleState", tc.entry))
			if err != nil {
				t.Fatal(err)
			}
			q := target.Query()
			if target.Scheme != "https" || target.Host != tc.host || target.Path != tc.path || target.Fragment != tc.fragment ||
				q.Get("appid") != config.CorpID || q.Get("agentid") != "1000001" || q.Get("scope") != tc.scope ||
				q.Get("login_type") != tc.loginType || q.Get("state") != "sampleState" ||
				q.Get("redirect_uri") != origin+"/api/oa/v1/auth/wecom/callback" || q.Get("corpsecret") != "" {
				t.Fatalf("unexpected authorization URL %q", target.String())
			}
		})
	}
}

func TestWeComMemberRefreshAndCache(t *testing.T) {
	var tokenRequests atomic.Int32
	var identityRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch r.URL.Path {
		case "/cgi-bin/gettoken":
			if r.Method != http.MethodGet || q.Get("corpid") != "oa-test" || q.Get("corpsecret") != "synthetic-secret" {
				t.Error("unexpected token request")
			}
			fmt.Fprintf(w, `{"errcode":0,"access_token":"token-%d","expires_in":7200}`, tokenRequests.Add(1))
		case "/cgi-bin/auth/getuserinfo":
			identityRequests.Add(1)
			switch {
			case q.Get("access_token") == "token-1":
				io.WriteString(w, `{"errcode":42001}`)
			case q.Get("access_token") != "token-2":
				t.Error("identity lookup did not use the refreshed token")
				http.Error(w, "unexpected access token", http.StatusBadRequest)
			case q.Get("code") == "external":
				io.WriteString(w, `{"errcode":0,"openid":"not-an-enterprise-member"}`)
			default:
				io.WriteString(w, `{"errcode":0,"userid":"Synthetic.Member"}`)
			}
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := testWeCom(t, server.URL)
	if member, err := client.Member(t.Context(), "member"); err != nil || member != "synthetic.member" {
		t.Fatalf("member = %q, err = %v", member, err)
	}
	if _, err := client.Member(t.Context(), "external"); !errors.Is(err, ErrNotMember) {
		t.Fatalf("external identity error = %v", err)
	}
	if tokenRequests.Load() != 2 || identityRequests.Load() != 3 {
		t.Fatalf("unexpected refresh/lookup counts %d/%d", tokenRequests.Load(), identityRequests.Load())
	}
}

func TestWeComDeliveryClassifiesProviderOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		httpStatus int
		want       DeliveryOutcome
	}{
		{"accepted", `{"errcode":0}`, http.StatusOK, DeliveryOutcome{Status: "ACCEPTED"}},
		{"invalid user", `{"errcode":0,"invaliduser":"synthetic.member"}`, http.StatusOK, DeliveryOutcome{Status: "FAILED"}},
		{"unlicensed user", `{"errcode":0,"unlicenseduser":"synthetic.member"}`, http.StatusOK, DeliveryOutcome{Status: "FAILED"}},
		{"rate limited", `{"errcode":45009}`, http.StatusOK, DeliveryOutcome{Status: "FAILED", Retry: true}},
		{"ambiguous gateway", "", http.StatusBadGateway, DeliveryOutcome{Status: "UNCERTAIN"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sendRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/cgi-bin/gettoken" {
					io.WriteString(w, `{"errcode":0,"access_token":"access-token","expires_in":7200}`)
					return
				}
				sendRequests.Add(1)
				if r.URL.Path != "/cgi-bin/message/send" || r.Method != http.MethodPost ||
					r.URL.Query().Get("access_token") != "access-token" {
					t.Error("unexpected message request")
				}
				var payload struct {
					ToUser    string            `json:"touser"`
					MsgType   string            `json:"msgtype"`
					AgentID   int64             `json:"agentid"`
					Text      map[string]string `json:"text"`
					Duplicate int               `json:"enable_duplicate_check"`
					Interval  int               `json:"duplicate_check_interval"`
				}
				decoder := json.NewDecoder(r.Body)
				decoder.DisallowUnknownFields()
				if err := decoder.Decode(&payload); err != nil || payload.ToUser != "synthetic.member" ||
					payload.MsgType != "text" || payload.AgentID != 1000001 || payload.Duplicate != 1 || payload.Interval != 1800 ||
					len(payload.Text) != 1 || payload.Text["content"] != "待处理。请进入 HR OA 查看。事件 9" {
					t.Errorf("unexpected message payload %+v, err = %v", payload, err)
				}
				w.WriteHeader(tc.httpStatus)
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			client := testWeCom(t, server.URL)
			if got := client.Send(context.Background(), "synthetic.member", "待处理", 9); got != tc.want {
				t.Fatalf("outcome = %+v, want %+v", got, tc.want)
			}
			if sendRequests.Load() != 1 {
				t.Fatalf("message attempts = %d, want 1", sendRequests.Load())
			}
		})
	}
}

func TestWeComNeverSendsRecipientSelectors(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request", http.StatusBadRequest)
	}))
	defer server.Close()
	client := testWeCom(t, server.URL)
	for _, selector := range []string{"@all", "@ALL", " @All\t", "synthetic-a|synthetic-b", "K"} {
		if _, ok := NormalizeMemberID(selector); ok {
			t.Errorf("accepted invalid member selector %q", selector)
		}
		if got := client.Send(t.Context(), selector, "待处理", 9); got != (DeliveryOutcome{Status: "FAILED"}) {
			t.Errorf("selector %q: unexpected outcome %+v", selector, got)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("requests = %d, want 0", requests.Load())
	}
}

func testWeCom(t *testing.T, base string) *WeCom {
	t.Helper()
	config := WeComConfig{
		Mode: "mock", CorpID: "oa-test", Secret: "synthetic-secret", AgentID: 1000001,
		MockAPIBase: base, MockAuthorizeURL: base + "/authorize", DeliverNotifications: true,
	}
	if err := config.Validate(base, false); err != nil {
		t.Fatal(err)
	}
	return NewWeCom(config, base)
}
