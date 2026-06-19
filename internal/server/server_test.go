package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthzOK(t *testing.T) {
	s, err := New(Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
}

func TestGithubReturns501(t *testing.T) {
	s, _ := New(Config{})
	req := httptest.NewRequest(http.MethodPost, "/webhook/github", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("want 501, got %d", rec.Code)
	}
}
