// Package server wires the webhook-router HTTP handlers.
//
// v0.1.0 is a scaffold: all webhook POST endpoints return 501 Not Implemented.
// The shape is fixed so follow-up PRs only fill in handler bodies.
package server

import (
	"log/slog"
	"net/http"
)

// Config bundles dependencies required at server construction.
type Config struct {
	Logger          *slog.Logger
	SubscribersPath string
	BridgeAddr      string
	BridgeCA        string
	ClientCert      string
	ClientKey       string
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
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
	s.routes()
	return s, nil
}

// Handler returns the http.Handler for the server.
func (s *Server) Handler() http.Handler { return s.mux }

func (s *Server) routes() {
	s.mux.HandleFunc("/healthz", s.healthz)
	s.mux.HandleFunc("/readyz", s.readyz)
	// Webhook sources. All return 501 in v0.1.0.
	s.mux.HandleFunc("/webhook/github", s.notImplemented("github"))
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

func (s *Server) notImplemented(source string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.cfg.Logger.Info("webhook received (stub)", "source", source, "method", r.Method, "ua", r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte("webhook-router v0.1.0 scaffold: handler not implemented\n"))
	}
}
