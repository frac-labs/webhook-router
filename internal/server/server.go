// Package server wires the webhook-router HTTP handlers.
//
// v0.2.0 (PR-4a): the /webhook/github endpoint now verifies the
// X-Hub-Signature-256 HMAC against a per-app shared secret loaded from
// the GITHUB_WEBHOOK_SECRET env var. Verified payloads still return 501
// (real handler bodies land in PR-4b); malformed/mismatched/missing
// signatures return 401.
//
// Plane and Mattermost remain 501 stubs.
package server

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/frac-labs/webhook-router/internal/hmacverify"
)

// Config bundles dependencies required at server construction.
type Config struct {
	Logger          *slog.Logger
	SubscribersPath string
	BridgeAddr      string
	BridgeCA        string
	ClientCert      string
	ClientKey       string

	// GitHubSecret is the shared secret used to verify GitHub webhook
	// X-Hub-Signature-256 headers. If empty, New reads GITHUB_WEBHOOK_SECRET
	// from the environment. A request hitting /webhook/github when the
	// secret is unset returns 503 (deliberately distinct from 401) so
	// misconfiguration is loud.
	GitHubSecret string
}

// Server is the webhook-router HTTP server.
type Server struct {
	cfg Config
	mux *http.ServeMux
}

// New constructs a Server. Returns an error if config is structurally invalid.
func New(cfg Config) (*Server, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.GitHubSecret == "" {
		cfg.GitHubSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

// Handler returns the http.Handler for the server.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.healthz)
	s.mux.HandleFunc("/readyz", s.readyz)
	s.mux.HandleFunc("/webhook/github", s.githubWebhook)
	// Plane and Mattermost stay 501 stubs in PR-4a; HMAC shape lands in PR-4b.
	s.mux.HandleFunc("/webhook/plane", s.notImplemented("plane"))
	s.mux.HandleFunc("/webhook/mattermost", s.notImplemented("mattermost"))
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) readyz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) githubWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.GitHubSecret == "" {
		s.cfg.Logger.Error("github webhook hit but secret not configured")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("github webhook secret not configured\n"))
		return
	}
	// Cap body at 25 MiB (GitHub max payload is 25 MiB per docs).
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 25<<20))
	if err != nil {
		s.cfg.Logger.Warn("github webhook body read failed", "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sig := r.Header.Get("X-Hub-Signature-256")
	if err := hmacverify.VerifyGitHub(s.cfg.GitHubSecret, sig, body); err != nil {
		// Loud at INFO for missing (operator likely mis-wired delivery),
		// WARN for mismatch (potential attack).
		switch {
		case errors.Is(err, hmacverify.ErrMissingHeader):
			s.cfg.Logger.Info("github webhook missing signature", "ua", r.Header.Get("User-Agent"))
		case errors.Is(err, hmacverify.ErrMalformedHeader):
			s.cfg.Logger.Warn("github webhook malformed signature", "sig_prefix", safePrefix(sig))
		case errors.Is(err, hmacverify.ErrMismatch):
			s.cfg.Logger.Warn("github webhook signature mismatch", "delivery", r.Header.Get("X-GitHub-Delivery"))
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid signature\n"))
		return
	}
	// Verified. Real fan-out lands in PR-4b; for now log + 501.
	s.cfg.Logger.Info("github webhook verified (stub)",
		"event", r.Header.Get("X-GitHub-Event"),
		"delivery", r.Header.Get("X-GitHub-Delivery"),
		"bytes", len(body),
	)
	w.WriteHeader(http.StatusNotImplemented)
	_, _ = w.Write([]byte("webhook-router PR-4a: signature ok, handler not yet implemented\n"))
}

// safePrefix returns at most the first 12 chars of s for safe logging.
func safePrefix(s string) string {
	if len(s) > 12 {
		return s[:12] + "..."
	}
	return s
}

func (s *Server) notImplemented(source string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.cfg.Logger.Info("webhook received (stub)", "source", source, "method", r.Method, "ua", r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte("webhook-router v0.2.0: handler not implemented\n"))
	}
}
