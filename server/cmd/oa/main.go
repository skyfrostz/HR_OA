// HR OA controlled runtime; upstream GVA main and development plugins are not mounted.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flipped-aurora/gin-vue-admin/server/oa/bootstrap"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	handler, err := bootstrap.New(bootstrap.Config{
		DSN: os.Getenv("OA_MYSQL_DSN"), StorageDir: env("OA_STORAGE_DIR", "./private/oa"),
		JWTSecret: os.Getenv("OA_JWT_SECRET"), DevSeed: os.Getenv("OA_DEV_SEED") == "true",
		SeedPassword: os.Getenv("OA_SEED_PASSWORD"), MockNotifyFail: os.Getenv("OA_MOCK_NOTIFY_FAIL") == "true",
	})
	if err != nil {
		return errors.New("OA initialization failed; check database, private directory and required environment configuration")
	}
	if closer, ok := handler.(interface{ Close() error }); ok {
		defer func() {
			if err := closer.Close(); err != nil {
				log.Print("OA background worker or database cleanup failed")
			}
		}()
	}
	srv := &http.Server{Addr: env("OA_ADDR", "127.0.0.1:8888"), Handler: handler,
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
	log.Printf("HR OA local-development API listening on %s", srv.Addr)
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

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
