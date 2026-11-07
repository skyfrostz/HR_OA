package main

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/bootstrap"
)

// Environment values are deliberately never included in validation errors.
type runtimeConfig struct {
	Environment    string
	Addr           string
	PublicOrigin   string
	TrustedProxies []string
	Bootstrap      bootstrap.Config
}

func loadRuntimeConfig(get func(string) string) (runtimeConfig, error) {
	value := func(key, fallback string) string {
		if v := get(key); v != "" {
			return v
		}
		return fallback
	}
	cfg := runtimeConfig{
		Environment:  value("OA_ENV", "development"),
		Addr:         value("OA_ADDR", "127.0.0.1:8888"),
		PublicOrigin: get("OA_PUBLIC_ORIGIN"),
		Bootstrap: bootstrap.Config{
			DSN: get("OA_MYSQL_DSN"), StorageDir: value("OA_STORAGE_DIR", "./private/oa"),
			JWTSecret: get("OA_JWT_SECRET"), SeedPassword: get("OA_SEED_PASSWORD"),
		},
	}
	if cfg.Environment != "development" && cfg.Environment != "production" {
		return cfg, errors.New("OA_ENV must be development or production")
	}
	for _, setting := range []struct {
		name   string
		target *bool
	}{
		{"OA_DEV_SEED", &cfg.Bootstrap.DevSeed},
		{"OA_MOCK_NOTIFY_FAIL", &cfg.Bootstrap.MockNotifyFail},
	} {
		v := value(setting.name, "false")
		if v != "true" && v != "false" {
			return cfg, fmt.Errorf("%s must be true or false", setting.name)
		}
		*setting.target = v == "true"
	}
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil || strings.ContainsAny(host, " \t\r\n") {
		return cfg, errors.New("OA_ADDR must be a valid host:port")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return cfg, errors.New("OA_ADDR must use a port between 1 and 65535")
	}
	if cfg.PublicOrigin != "" {
		u, err := url.Parse(cfg.PublicOrigin)
		if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && u.Scheme != "http") {
			return cfg, errors.New("OA_PUBLIC_ORIGIN must be an HTTP(S) origin without credentials, path, query or fragment")
		}
		if cfg.Environment == "production" && u.Scheme != "https" {
			return cfg, errors.New("OA_PUBLIC_ORIGIN must use HTTPS in production")
		}
		cfg.PublicOrigin = strings.TrimSuffix(cfg.PublicOrigin, "/")
	}
	if raw := get("OA_TRUSTED_PROXIES"); raw != "" {
		for _, item := range strings.Split(raw, ",") {
			item = strings.TrimSpace(item)
			prefix, err := netip.ParsePrefix(item)
			if err != nil {
				ip, ipErr := netip.ParseAddr(item)
				if ipErr != nil || ip.Zone() != "" || ip.IsUnspecified() {
					return cfg, errors.New("OA_TRUSTED_PROXIES must contain explicit IP addresses or CIDRs")
				}
				prefix = netip.PrefixFrom(ip, ip.BitLen())
			}
			if prefix.Bits() == 0 || (prefix.Addr().Is4In6() && prefix.Bits() <= 96) {
				return cfg, errors.New("OA_TRUSTED_PROXIES must not trust all addresses")
			}
			cfg.TrustedProxies = append(cfg.TrustedProxies, prefix.Masked().String())
		}
	}
	if cfg.Bootstrap.DSN == "" || len(cfg.Bootstrap.JWTSecret) < 32 {
		return cfg, errors.New("OA_MYSQL_DSN and OA_JWT_SECRET (at least 32 bytes) are required")
	}
	if cfg.Bootstrap.DevSeed && (len(cfg.Bootstrap.SeedPassword) < 12 || len(cfg.Bootstrap.SeedPassword) > 72) {
		return cfg, errors.New("OA_SEED_PASSWORD must contain 12 to 72 bytes when OA_DEV_SEED is enabled")
	}
	if cfg.Environment == "production" {
		if cfg.PublicOrigin == "" {
			return cfg, errors.New("OA_PUBLIC_ORIGIN is required in production")
		}
		if cfg.Bootstrap.DevSeed || cfg.Bootstrap.MockNotifyFail {
			return cfg, errors.New("development seeding and simulated notification failures are forbidden in production")
		}
		if len(cfg.Bootstrap.JWTSecret) < 48 || len(uniqueBytes(cfg.Bootstrap.JWTSecret)) < 12 {
			return cfg, errors.New("OA_JWT_SECRET must be a strong random secret of at least 48 bytes in production")
		}
	}
	cfg.Bootstrap.Production = cfg.Environment == "production"
	cfg.Bootstrap.PublicOrigin = cfg.PublicOrigin
	cfg.Bootstrap.TrustedProxies = cfg.TrustedProxies
	cfg.Bootstrap.AttachmentWriteBackend = value("OA_ATTACHMENT_WRITE_BACKEND", "local")
	cfg.Bootstrap.BootstrapAdminUsername = get("OA_BOOTSTRAP_ADMIN_USERNAME")
	cfg.Bootstrap.BootstrapAdminPassword = get("OA_BOOTSTRAP_ADMIN_PASSWORD")
	cfg.Bootstrap.WeCom = bootstrap.WeComConfig{
		Mode: value("OA_WECOM_MODE", "disabled"), CorpID: get("OA_WECOM_CORP_ID"),
		Secret: get("OA_WECOM_SECRET"), MockAPIBase: get("OA_WECOM_MOCK_API_BASE"),
		MockAuthorizeURL: get("OA_WECOM_MOCK_AUTHORIZE_URL"),
	}
	if raw := get("OA_WECOM_AGENT_ID"); raw != "" {
		cfg.Bootstrap.WeCom.AgentID, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || cfg.Bootstrap.WeCom.AgentID <= 0 {
			return cfg, errors.New("OA_WECOM_AGENT_ID must be a positive integer")
		}
	}
	cfg.Bootstrap.OSS = bootstrap.OSSConfig{
		Region: get("OA_OSS_REGION"), Bucket: get("OA_OSS_BUCKET"), Endpoint: get("OA_OSS_ENDPOINT"),
		Prefix: value("OA_OSS_PREFIX", "hr-oa"), AccessKeyID: get("OA_OSS_ACCESS_KEY_ID"),
		AccessKeySecret: get("OA_OSS_ACCESS_KEY_SECRET"), SecurityToken: get("OA_OSS_SECURITY_TOKEN"),
	}
	for _, setting := range []struct {
		name   string
		target *bool
	}{
		{"OA_WECOM_DELIVER_NOTIFICATIONS", &cfg.Bootstrap.WeCom.DeliverNotifications},
		{"OA_OSS_ENABLED", &cfg.Bootstrap.OSS.Enabled},
		{"OA_OSS_MOCK", &cfg.Bootstrap.OSS.Mock},
		{"OA_OSS_USE_CNAME", &cfg.Bootstrap.OSS.UseCName},
		{"OA_CONTRACT_REMINDER_SCHEDULE", &cfg.Bootstrap.ContractReminderSchedule},
	} {
		v := value(setting.name, "false")
		if v != "true" && v != "false" {
			return cfg, fmt.Errorf("%s must be true or false", setting.name)
		}
		*setting.target = v == "true"
	}
	if raw := get("OA_CONTRACT_REMINDER_DAYS"); raw != "" {
		cfg.Bootstrap.ContractReminderDays, err = strconv.Atoi(raw)
		if err != nil || cfg.Bootstrap.ContractReminderDays < 0 || cfg.Bootstrap.ContractReminderDays > 90 {
			return cfg, errors.New("OA_CONTRACT_REMINDER_DAYS must be between 0 and 90")
		}
	}
	return cfg, nil
}

func uniqueBytes(s string) map[byte]struct{} {
	values := make(map[byte]struct{})
	for i := range s {
		values[s[i]] = struct{}{}
	}
	return values
}
