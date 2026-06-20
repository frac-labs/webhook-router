// Package server wires the webhook-router HTTP handlers.
//
// v0.3.0 (PR-4c): /webhook/plane joins /webhook/github as fully verified
// and fanned out. Plane signs with HMAC-SHA256 in X-Plane-Signature
// (raw hex, no prefix); secret from PLANE_WEBHOOK_SECRET. 503 if secret
// unset, 401 on missing/malformed/mismatch, 200 on verify-OK.
//
// Mattermost remains a 501 stub.
package server

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"

	"github.com/frac-labs/webhook-router/internal/fanout"
	"github.com/frac-labs/webhook-router/internal/hmacverify"
	"github.com/frac-labs/webhook-router/internal/normalize"
	"github.com/frac-labs/webhook-router/internal/subscribers"
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

	// PlaneSecret is the shared secret used to verify Plane webhook
	// X-Plane-Signature headers. If empty, New reads PLANE_WEBHOOK_SECRET
	// from the environment. Same 503-on-missing convention as GitHub.
	PlaneSecret string
}

// Server is the webhook-router HTTP server.
type Server struct {
	cfg        Config
	mux        *http.ServeMux
	dispatcher *fanout.Dispatcher
}

// New constructs a Server. Returns an error if config is structurally invalid.
func New(cfg Config) (*Server, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.GitHubSecret == "" {
		cfg.GitHubSecret = os.Getenv("GITHUB_WEBHOOK_SECRET")
	}
	if cfg.PlaneSecret == "" {
		cfg.PlaneSecret = os.Getenv("PLANE_WEBHOOK_SECRET")
	}
	subs, err := subscribers.Load(cfg.SubscribersPath)
	if err != nil {
		return nil, err
	}
	cfg.Logger.Info("subscribers loaded", "path", cfg.SubscribersPath, "count", len(subs.Subscribers))
	s := &Server{
		cfg:        cfg,
		mux:        http.NewServeMux(),
		dispatcher: fanout.New(subs.Subscribers, cfg.Logger),
	}
	s.routes()
	return s, nil
}

// Handler returns the http.Handler for the server.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.healthz)
	s.mux.HandleFunc("/readyz", s.readyz)
	s.mux.HandleFunc("/webhook/github", s.githubWebhook)
	s.mux.HandleFunc("/webhook/plane", s.planeWebhook)
	// Mattermost remains a 501 stub.
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
	// Verified. Normalize + fan out.
	eventHeader := r.Header.Get("X-GitHub-Event")
	ev, err := normalize.GitHub(eventHeader, body)
	if err != nil {
		s.cfg.Logger.Warn("github webhook normalize failed",
			"event", eventHeader, "err", err)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("could not parse webhook body\n"))
		return
	}
	attempts := s.dispatcher.Dispatch(r.Context(), ev)
	s.cfg.Logger.Info("github webhook verified",
		"event", ev.EventName(),
		"delivery", r.Header.Get("X-GitHub-Delivery"),
		"actor", ev.Actor,
		"target", ev.Target,
		"bytes", len(body),
		"fanout_attempts", attempts,
	)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
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

func (s *Server) planeWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.PlaneSecret == "" {
		s.cfg.Logger.Error("plane webhook hit but secret not configured")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("plane webhook secret not configured\n"))
		return
	}
	// Cap body at 5 MiB — Plane payloads are tiny vs GitHub.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 5<<20))
	if err != nil {
		s.cfg.Logger.Warn("plane webhook body read failed", "err", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	sig := r.Header.Get("X-Plane-Signature")
	if err := hmacverify.VerifyPlane(s.cfg.PlaneSecret, sig, body); err != nil {
		switch {
		case errors.Is(err, hmacverify.ErrMissingHeader):
			s.cfg.Logger.Info("plane webhook missing signature", "ua", r.Header.Get("User-Agent"))
		case errors.Is(err, hmacverify.ErrMalformedHeader):
			s.cfg.Logger.Warn("plane webhook malformed signature", "sig_prefix", safePrefix(sig))
		case errors.Is(err, hmacverify.ErrMismatch):
			s.cfg.Logger.Warn("plane webhook signature mismatch", "delivery", r.Header.Get("X-Plane-Delivery"))
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid signature\n"))
		return
	}
	ev, err := normalize.Plane(body)
	if err != nil {
		s.cfg.Logger.Warn("plane webhook normalize failed",
			"event", r.Header.Get("X-Plane-Event"), "err", err)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("could not parse webhook body\n"))
		return
	}
	attempts := s.dispatcher.Dispatch(r.Context(), ev)
	s.cfg.Logger.Info("plane webhook verified",
		"event", ev.EventName(),
		"delivery", r.Header.Get("X-Plane-Delivery"),
		"actor", ev.Actor,
		"target", ev.Target,
		"bytes", len(body),
		"fanout_attempts", attempts,
	)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}
