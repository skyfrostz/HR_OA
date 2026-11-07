package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/bootstrap"
)

func TestStartupDiagnosticsUseOnlyFixedStages(t *testing.T) {
	if !strings.Contains(startupFailure(errors.New("OA database migration failed")).Error(), "stage=database-migration") {
		t.Fatal("known stage missing")
	}
	for _, value := range []string{"open /private/customer/payroll: denied", "https://secret-user:private-token@provider.invalid", "mysql secret-password"} {
		message := startupFailure(errors.New(value)).Error()
		if strings.Contains(message, value) || !strings.Contains(message, "stage=runtime-configuration-or-storage") {
			t.Fatal("untrusted startup detail reached diagnostics")
		}
	}
}

func runtimeTestEnv() map[string]string {
	return map[string]string{
		"OA_MYSQL_DSN":  "test:private-database-secret@tcp(localhost:3306)/test",
		"OA_JWT_SECRET": "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN",
	}
}

func TestBootstrapValidationNeverIncludesEnvironmentSecrets(t *testing.T) {
	const sentinel = "private-sentinel-do-not-print"
	for _, tc := range []struct {
		name   string
		update func(*bootstrap.Config)
	}{
		{"DSN", func(c *bootstrap.Config) { c.DSN = sentinel + "%invalid" }},
		{"WeCom", func(c *bootstrap.Config) {
			c.WeCom = bootstrap.WeComConfig{Mode: "mock", Secret: sentinel, MockAPIBase: "https://" + sentinel + ".invalid"}
		}},
		{"OSS", func(c *bootstrap.Config) {
			c.OSS = bootstrap.OSSConfig{Enabled: true, Region: "cn-hangzhou", Bucket: "qa-bucket", Endpoint: "https://user:" + sentinel + "@invalid.example", AccessKeyID: "qa-access", AccessKeySecret: sentinel}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := runtimeTestEnv()
			cfg, err := loadRuntimeConfig(func(k string) string { return env[k] })
			if err != nil {
				t.Fatal(err)
			}
			tc.update(&cfg.Bootstrap)
			err = bootstrap.ValidateConfig(cfg.Bootstrap)
			if err == nil {
				t.Fatal("invalid configuration accepted")
			}
			if strings.Contains(err.Error(), sentinel) {
				t.Fatal("provider validation exposed a secret")
			}
		})
	}
}

func TestRuntimeConfigurationRejectsUnsafeProduction(t *testing.T) {
	for _, tc := range []struct{ name, key, value string }{
		{"missing origin", "OA_PUBLIC_ORIGIN", ""},
		{"unencrypted origin", "OA_PUBLIC_ORIGIN", "http://oa.example.test"},
		{"seed", "OA_DEV_SEED", "true"},
		{"mock failure", "OA_MOCK_NOTIFY_FAIL", "true"},
		{"weak secret", "OA_JWT_SECRET", strings.Repeat("x", 64)},
		{"ipv4 all proxies", "OA_TRUSTED_PROXIES", "0.0.0.0/0"},
		{"ipv6 all proxies", "OA_TRUSTED_PROXIES", "::/0"},
		{"mapped all proxies", "OA_TRUSTED_PROXIES", "::ffff:0.0.0.0/96"},
		{"invalid proxy", "OA_TRUSTED_PROXIES", "10.1.0.2,evil.example"},
		{"invalid bool", "OA_DEV_SEED", "yes"},
		{"invalid delivery toggle", "OA_WECOM_DELIVER_NOTIFICATIONS", "1"},
		{"invalid bucket toggle", "OA_OSS_ENABLED", "yes"},
		{"invalid agent", "OA_WECOM_AGENT_ID", "0"},
		{"invalid reminder days", "OA_CONTRACT_REMINDER_DAYS", "91"},
		{"invalid mode", "OA_ENV", "prod"},
		{"credentials in origin", "OA_PUBLIC_ORIGIN", "https://user:private-value@oa.example.test"},
		{"query in origin", "OA_PUBLIC_ORIGIN", "https://oa.example.test?secret=private-value"},
		{"path in origin", "OA_PUBLIC_ORIGIN", "https://oa.example.test/oa.html"},
		{"invalid port", "OA_ADDR", "127.0.0.1:99999"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := runtimeTestEnv()
			env["OA_ENV"], env["OA_PUBLIC_ORIGIN"] = "production", "https://oa.example.test"
			env["OA_SEED_PASSWORD"] = "private-seed-value"
			env[tc.key] = tc.value
			_, err := loadRuntimeConfig(func(k string) string { return env[k] })
			if err == nil {
				t.Fatal("unsafe configuration accepted")
			}
			for _, secret := range []string{"private-database-secret", "private-seed-value", "private-value", env["OA_JWT_SECRET"]} {
				if strings.Contains(err.Error(), secret) {
					t.Fatal("configuration error disclosed a secret")
				}
			}
		})
	}
}

func TestProviderConfigurationIsServerSideAndExplicit(t *testing.T) {
	env := runtimeTestEnv()
	for k, v := range map[string]string{
		"OA_PUBLIC_ORIGIN": "http://127.0.0.1:8080", "OA_WECOM_MODE": "mock",
		"OA_WECOM_CORP_ID": "qa-corp", "OA_WECOM_AGENT_ID": "100001",
		"OA_WECOM_SECRET": "test-private-app-secret", "OA_WECOM_MOCK_API_BASE": "http://127.0.0.1:8989",
		"OA_WECOM_MOCK_AUTHORIZE_URL":    "http://127.0.0.1:8989/authorize",
		"OA_WECOM_DELIVER_NOTIFICATIONS": "true", "OA_OSS_ENABLED": "true", "OA_OSS_MOCK": "true",
		"OA_OSS_REGION": "cn-hangzhou", "OA_OSS_BUCKET": "qa-private-bucket",
		"OA_OSS_ENDPOINT": "http://127.0.0.1:8990", "OA_OSS_ACCESS_KEY_ID": "qa-access-id",
		"OA_OSS_ACCESS_KEY_SECRET": "qa-private-access-secret", "OA_ATTACHMENT_WRITE_BACKEND": "oss",
		"OA_BOOTSTRAP_ADMIN_USERNAME": "initial-admin", "OA_BOOTSTRAP_ADMIN_PASSWORD": "qa-initial-password",
		"OA_CONTRACT_REMINDER_SCHEDULE": "true", "OA_CONTRACT_REMINDER_DAYS": "30",
	} {
		env[k] = v
	}
	cfg, err := loadRuntimeConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Bootstrap.Production || cfg.Bootstrap.WeCom.AgentID != 100001 || !cfg.Bootstrap.WeCom.DeliverNotifications ||
		!cfg.Bootstrap.OSS.Mock || cfg.Bootstrap.OSS.Prefix != "hr-oa" || cfg.Bootstrap.AttachmentWriteBackend != "oss" ||
		!cfg.Bootstrap.ContractReminderSchedule || cfg.Bootstrap.ContractReminderDays != 30 ||
		cfg.Bootstrap.BootstrapAdminUsername != "initial-admin" || cfg.Bootstrap.BootstrapAdminPassword != env["OA_BOOTSTRAP_ADMIN_PASSWORD"] {
		t.Fatal("provider configuration wiring is incomplete")
	}
}

func TestRuntimeConfigurationDefaultsAndExplicitProxy(t *testing.T) {
	env := runtimeTestEnv()
	cfg, err := loadRuntimeConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:8888" || cfg.Environment != "development" || len(cfg.TrustedProxies) != 0 || cfg.Bootstrap.DevSeed {
		t.Fatal("unsafe runtime defaults")
	}
	env["OA_ENV"], env["OA_PUBLIC_ORIGIN"] = "production", "https://oa.example.test/"
	env["OA_TRUSTED_PROXIES"] = "192.0.2.4, 2001:db8:1::/64"
	cfg, err = loadRuntimeConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicOrigin != "https://oa.example.test" || len(cfg.TrustedProxies) != 2 || cfg.TrustedProxies[0] != "192.0.2.4/32" {
		t.Fatal("explicit configuration was not normalized")
	}
}
