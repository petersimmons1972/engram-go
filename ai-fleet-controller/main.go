package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/petersimmons1972/ai-fleet-controller/internal"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	k8s, err := internal.NewK8sClient()
	if err != nil {
		slog.Error("k8s client", "err", err)
		os.Exit(1)
	}

	store := internal.NewStore()
	srv := internal.NewServer(k8s, store)

	addr := os.Getenv("LISTEN_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	slog.Info("ai-fleet-controller starting", "addr", addr)
	if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
}
