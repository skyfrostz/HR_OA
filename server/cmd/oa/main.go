// HR OA controlled runtime; upstream GVA main and development plugins are not mounted.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/bootstrap"
	"github.com/flipped-aurora/gin-vue-admin/server/oa/service"
	"github.com/gin-gonic/gin"
)

// Set at packaging time with -ldflags; never derive a version from runtime secrets.
var buildVersion = "development"

func main() {
	if err := run(); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			log.Fatal(err)
		}
	}
}

func run() error {
	opts, err := parseOptions(os.Args[1:], os.Stderr)
	if err != nil {
		return err
	}
	if opts.version {
		fmt.Fprintln(os.Stdout, buildVersion)
		return nil
	}
	cfg, err := loadRuntimeConfig(os.Getenv)
	if err != nil {
		return err
	}
	if err := bootstrap.ValidateConfig(cfg.Bootstrap); err != nil {
		// ValidateConfig and provider validators return fixed field names only.
		return fmt.Errorf("OA configuration invalid: %w", err)
	}
	if opts.checkConfig {
		fmt.Fprintln(os.Stdout, "Configuration valid; database and providers have not been contacted")
		return nil
	}
	cfg.Bootstrap.DisableWorkers = opts.migrateAttachments
	// Gin's debug route table writes to stdout. Keep maintenance output a
	// single machine-readable JSON result and normal startup logs redacted.
	gin.SetMode(gin.ReleaseMode)
	handler, err := bootstrap.New(cfg.Bootstrap)
	if err != nil {
		return startupFailure(err)
	}
	if closer, ok := handler.(interface{ Close() error }); ok {
		defer func() {
			if err := closer.Close(); err != nil {
				log.Print("OA background worker or database cleanup failed")
			}
		}()
	}
	if opts.migrateAttachments {
		migrator, ok := handler.(interface {
			MigrateAttachments(context.Context, uint, int, bool) (service.AttachmentMigrationResult, error)
		})
		if !ok {
			return errors.New("attachment migration is unavailable")
		}
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()
		ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
		defer cancel()
		result, err := migrator.MigrateAttachments(ctx, opts.afterID, opts.limit, opts.apply)
		if outputErr := json.NewEncoder(os.Stdout).Encode(result); outputErr != nil {
			return outputErr
		}
		if err != nil {
			return errors.New("attachment migration interrupted; inspect the partial report before resuming; no source files were removed")
		}
		if result.Verified != result.Examined {
			return errors.New("some attachments could not be verified or migrated; retry failed IDs before advancing the cursor; no source files were removed")
		}
		return nil
	}
	srv := &http.Server{Addr: cfg.Addr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			_ = srv.Close()
		}
	}()
	log.Printf("HR OA API started (mode=%s version=%s)", cfg.Environment, buildVersion)
	err = srv.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		stop()
	}
	// ListenAndServe returns as soon as shutdown begins; keep the DB and outbox
	// alive until active HTTP handlers have drained or the deadline has elapsed.
	<-shutdownDone
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return errors.New("OA HTTP server failed")
	}
	return nil
}

// Only these fixed stage labels reach logs. In particular, filesystem and
// provider errors must not disclose their paths, URLs, DSNs or credentials.
func startupFailure(err error) error {
	stages := map[string]string{
		"OA database connection failed":                                 "database-connection",
		"OA database migration failed":                                  "database-migration",
		"OA development seed failed":                                    "development-seed",
		"OA administrator initialization failed":                        "administrator-initialization",
		"OA department backfill failed":                                 "department-backfill",
		"OA authentication initialization failed":                       "authentication",
		"OA router initialization failed":                               "router",
		"OSS bucket ACL is not verified private":                        "oss-private-acl",
		"OSS bucket public access block is not verified":                "oss-public-access-block",
		"OSS bucket must have versioning unset for no-overwrite writes": "oss-versioning",
	}
	stage := stages[err.Error()]
	if stage == "" {
		stage = "runtime-configuration-or-storage"
	}
	return fmt.Errorf("OA initialization failed (stage=%s); check the private server configuration", stage)
}
