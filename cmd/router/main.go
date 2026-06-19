// Command router is the webhook-router HTTP receiver.
//
// Verifies incoming webhook HMAC signatures, normalizes to a canonical event
// shape, and fans out to subscribers (Mattermost, Plane, Hermes, Frac) via the
// harness-bridge for any operations needing a GH App token.
//
// v0.1.0 is a scaffold stub: handlers return 501. Real logic in follow-ups.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/frac-labs/webhook-router/internal/server"
)

func main() {
	var (
		listen      = flag.String("listen", ":8080", "HTTP listen address")
		subsPath    = flag.String("subscribers", "/etc/webhook-router/subscribers.yaml", "subscribers config path")
		bridgeAddr  = flag.String("bridge-addr", "harness-bridge.fractura-bridge.svc.cluster.local:8443", "harness-bridge gRPC address")
		bridgeCA    = flag.String("bridge-ca", "/run/secrets/harness-ca/ca.crt", "harness-bridge server CA bundle (PEM)")
		clientCert  = flag.String("client-cert", "/run/secrets/client/tls.crt", "client TLS cert for bridge (PEM)")
		clientKey   = flag.String("client-key", "/run/secrets/client/tls.key", "client TLS key for bridge (PEM)")
	)
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	srv, err := server.New(server.Config{
		Logger:          logger,
		SubscribersPath: *subsPath,
		BridgeAddr:      *bridgeAddr,
		BridgeCA:        *bridgeCA,
		ClientCert:      *clientCert,
		ClientKey:       *clientKey,
	})
	if err != nil {
		logger.Error("server init failed", "err", err)
		os.Exit(1)
	}

	hs := &http.Server{
		Addr:              *listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	go func() {
		<-ctx.Done()
		logger.Info("shutting down")
		shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = hs.Shutdown(shutdownCtx)
	}()

	logger.Info("webhook-router listening", "addr", *listen, "subscribers", *subsPath, "bridge", *bridgeAddr)
	if err := hs.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("serve failed", "err", err)
		os.Exit(1)
	}
}
