package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/repyg/brightest_teeth/backend/internal/inference"
	"github.com/repyg/brightest_teeth/backend/internal/service"
	"github.com/repyg/brightest_teeth/backend/internal/storage"
)

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func run() error {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		client := &http.Client{Timeout: 4 * time.Second}
		res, err := client.Get("http://127.0.0.1:8080/readyz")
		if err != nil {
			return err
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return fmt.Errorf("not ready: %d", res.StatusCode)
		}
		return nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	version := env("MODEL_VERSION", "reid-resnet50-v1")
	db, err := storage.OpenPostgres(startup, env("DATABASE_URL", "postgres://brightest_teeth:brightest_teeth@localhost:5432/brightest_teeth?sslmode=disable"), version)
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	defer db.Pool.Close()
	blobs, err := storage.NewS3(env("S3_ENDPOINT", "http://localhost:9000"), env("S3_REGION", "us-east-1"), env("S3_BUCKET", "vehicle-photos"), env("S3_ACCESS_KEY_ID", "minioadmin"), env("S3_SECRET_ACCESS_KEY", "minioadmin"))
	if err != nil {
		return err
	}
	ml, err := inference.New(env("ML_URL", "http://localhost:8000"), version)
	if err != nil {
		return err
	}
	svc := &service.Service{Repo: db, Blobs: blobs, ML: ml, Bucket: blobs.Bucket, ModelVersion: version}
	handler, err := service.NewHTTP(svc, strings.Split(env("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:3000,http://127.0.0.1:5173,http://127.0.0.1:3000"), ","))
	if err != nil {
		return err
	}
	server := &http.Server{Addr: env("HTTP_ADDR", ":8080"), Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 65 * time.Second, WriteTimeout: 70 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	slog.Info("API listening", "address", server.Addr, "model_version", version)
	select {
	case err := <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return err
		}
	}
	return nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}
