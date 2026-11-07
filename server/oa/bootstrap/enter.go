package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/api"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/integration"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/router"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/service"
	driver "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func New(config Config) (http.Handler, error) {
	if err := ValidateConfig(config); err != nil {
		return nil, err
	}
	dir, err := prepareStorage(config.StorageDir)
	if err != nil {
		return nil, err
	}
	dsn, err := driver.ParseDSN(config.DSN)
	if err != nil || dsn.DBName == "" {
		return nil, errors.New("invalid OA MySQL DSN")
	}
	dsn.ParseTime = true
	dsn.Loc = time.UTC
	if dsn.Timeout == 0 {
		dsn.Timeout = 5 * time.Second
	}
	if dsn.ReadTimeout == 0 {
		dsn.ReadTimeout = 15 * time.Second
	}
	if dsn.WriteTimeout == 0 {
		dsn.WriteTimeout = 15 * time.Second
	}
	connector, err := driver.NewConnector(dsn)
	if err != nil {
		return nil, errors.New("invalid OA MySQL connector")
	}
	pool := sql.OpenDB(connector)
	pool.SetMaxOpenConns(20)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(3 * time.Minute)
	started := false
	defer func() {
		if !started {
			_ = pool.Close()
		}
	}()
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: pool}), &gorm.Config{
		Logger:  logger.Default.LogMode(logger.Silent),
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, errors.New("OA database connection failed")
	}
	if err := migrate(db); err != nil {
		return nil, errors.New("OA database migration failed")
	}
	if config.DevSeed {
		if err := seed(db, config.SeedPassword); err != nil {
			return nil, errors.New("OA development seed failed")
		}
	}

	if err := initializeAdministrator(db, config.BootstrapAdminUsername, config.BootstrapAdminPassword, config.Production); err != nil {
		return nil, errors.New("OA administrator initialization failed")
	}

	if err := backfillDepartments(db); err != nil {
		return nil, errors.New("OA department backfill failed")
	}

	dummyHash, err := bcrypt.GenerateFromPassword([]byte("OA timing placeholder"), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("OA authentication initialization failed")
	}
	wecom := integration.NewWeCom(config.WeCom, config.PublicOrigin)
	oss := integration.NewOSS(config.OSS)
	if oss != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		err := oss.ValidateBucket(ctx)
		cancel()
		if err != nil {
			return nil, err
		}
	}
	reminderDays := 0
	if config.ContractReminderSchedule {
		reminderDays = config.ContractReminderDays
		if reminderDays == 0 {
			reminderDays = 30
		}
	}
	svc := service.New(db, dir, []byte(config.JWTSecret), dummyHash)
	svc.ConfigureIntegrations(wecom, oss, config.AttachmentWriteBackend, reminderDays)
	handler, err := router.NewWithTrustedProxies(api.New(svc), config.TrustedProxies)
	if err != nil {
		return nil, errors.New("OA router initialization failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &oaHandler{Handler: handler, pool: pool, cancel: cancel, done: make(chan struct{}), service: svc}
	if config.DisableWorkers {
		close(runtime.done)
	} else {
		go func() {
			defer close(runtime.done)
			var workers sync.WaitGroup
			for _, run := range []func(){
				func() { svc.RunMockOutbox(ctx, config.MockNotifyFail) },
				func() { svc.RunProviderOutbox(ctx) },
				func() { svc.RunCleanup(ctx) },
				func() { svc.RunContractReminders(ctx) },
			} {
				workers.Add(1)
				go func() {
					defer workers.Done()
					run()
				}()
			}
			workers.Wait()
		}()
	}

	started = true
	return runtime, nil
}

type oaHandler struct {
	http.Handler
	pool      *sql.DB
	service   *service.Service
	cancel    context.CancelFunc
	done      chan struct{}
	closeOnce sync.Once
	closeErr  error
}

func (h *oaHandler) Close() error {
	h.closeOnce.Do(func() {
		h.cancel()
		<-h.done
		h.closeErr = h.pool.Close()
	})
	return h.closeErr
}

func (h *oaHandler) MigrateAttachments(ctx context.Context, afterID uint, limit int, apply bool) (service.AttachmentMigrationResult, error) {
	return h.service.MigrateAttachments(ctx, afterID, limit, apply)
}
