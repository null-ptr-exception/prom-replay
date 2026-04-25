package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	minioclient "github.com/rophy/prom-replay/replay-manager/internal/minio"
	"github.com/rophy/prom-replay/replay-manager/internal/server"
	"github.com/rophy/prom-replay/replay-manager/internal/vm"
)

func main() {
	vmURL := envOr("VM_URL", "http://localhost:8428")
	minioEndpoint := envOr("MINIO_ENDPOINT", "localhost:9000")
	minioAccessKey := envOr("MINIO_ACCESS_KEY", "minioadmin")
	minioSecretKey := envOr("MINIO_SECRET_KEY", "minioadmin")
	minioBucket := envOr("MINIO_BUCKET", "prom-replay")
	listenAddr := envOr("LISTEN_ADDR", ":8080")

	vmClient := vm.NewClient(vmURL)

	mc, err := minioclient.NewClient(minioEndpoint, minioAccessKey, minioSecretKey, minioBucket, false)
	if err != nil {
		slog.Error("failed to create minio client", "error", err)
		os.Exit(1)
	}

	if err := mc.EnsureBucket(context.Background()); err != nil {
		slog.Error("failed to ensure bucket", "error", err)
		os.Exit(1)
	}

	srv := server.New(vmClient, mc)

	slog.Info("starting replay manager", "addr", listenAddr, "vm_url", vmURL, "minio_endpoint", minioEndpoint)
	if err := http.ListenAndServe(listenAddr, srv); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
