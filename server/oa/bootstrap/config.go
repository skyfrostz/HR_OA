package bootstrap

import (
	"errors"
	"net/netip"
	"regexp"
	"strings"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/integration"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/model"
	driver "github.com/go-sql-driver/mysql"
)

type WeComConfig = integration.WeComConfig
type OSSConfig = integration.OSSConfig

type Config struct {
	DSN                      string
	StorageDir               string
	JWTSecret                string
	SeedPassword             string
	DevSeed                  bool
	MockNotifyFail           bool
	Production               bool
	PublicOrigin             string
	TrustedProxies           []string
	WeCom                    WeComConfig
	OSS                      OSSConfig
	AttachmentWriteBackend   string
	DisableWorkers           bool
	BootstrapAdminUsername   string
	BootstrapAdminPassword   string
	ContractReminderSchedule bool
	ContractReminderDays     int
}

var bootstrapUsername = regexp.MustCompile(`^[A-Za-z0-9._-]{3,64}$`)

// ValidateConfig is pure and does not contact a database or provider.
func ValidateConfig(config Config) error {
	if len(config.JWTSecret) < 32 {
		return errors.New("invalid OA_JWT_SECRET")
	}
	dsn, err := driver.ParseDSN(config.DSN)
	if err != nil || dsn.DBName == "" {
		return errors.New("invalid OA_MYSQL_DSN")
	}
	if config.DevSeed && (len(config.SeedPassword) < 12 || len(config.SeedPassword) > 72) {
		return errors.New("invalid OA_SEED_PASSWORD")
	}
	if config.PublicOrigin != "" && !integration.ValidateOrigin(config.PublicOrigin, config.Production) {
		return errors.New("invalid OA_PUBLIC_ORIGIN")
	}
	if config.Production {
		if config.PublicOrigin == "" || dsn.TLSConfig != "true" || config.DevSeed || config.MockNotifyFail ||
			len(config.JWTSecret) < 48 || uniqueSecretBytes(config.JWTSecret) < 12 {
			return errors.New("invalid OA production origin, TLS, secret, or development flags")
		}
	}
	if config.BootstrapAdminUsername != "" || config.BootstrapAdminPassword != "" {
		password := config.BootstrapAdminPassword
		if config.DevSeed || !bootstrapUsername.MatchString(config.BootstrapAdminUsername) ||
			len(password) < 16 || len(password) > 72 || uniqueSecretBytes(password) < 8 || strings.ContainsRune(password, 0) {
			return errors.New("invalid OA_BOOTSTRAP_ADMIN_USERNAME or OA_BOOTSTRAP_ADMIN_PASSWORD")
		}
	}
	for _, proxy := range config.TrustedProxies {
		if _, err := netip.ParseAddr(proxy); err == nil {
			continue
		}
		prefix, err := netip.ParsePrefix(proxy)
		if err != nil || prefix.Bits() == 0 || (prefix.Addr().Is4In6() && prefix.Bits() <= 96) {
			return errors.New("invalid OA_TRUSTED_PROXIES")
		}
	}
	if err := config.WeCom.Validate(config.PublicOrigin, config.Production); err != nil {
		return err
	}
	if err := config.OSS.Validate(config.Production); err != nil {
		return err
	}
	switch config.AttachmentWriteBackend {
	case "", model.StorageLocal:
	case model.StorageOSS:
		if !config.OSS.Enabled {
			return errors.New("OA_ATTACHMENT_WRITE_BACKEND requires enabled OSS")
		}
	default:
		return errors.New("invalid OA_ATTACHMENT_WRITE_BACKEND")
	}
	if config.ContractReminderDays < 0 || config.ContractReminderDays > 90 {
		return errors.New("invalid OA_CONTRACT_REMINDER_DAYS")
	}
	return nil
}

func uniqueSecretBytes(value string) int {
	var seen [256]bool
	var unique int
	for i := 0; i < len(value); i++ {
		if !seen[value[i]] {
			seen[value[i]] = true
			unique++
		}
	}
	return unique
}
