package server

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
)

func ghSig(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

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

func TestGithubNoSecret_503(t *testing.T) {
	s, _ := New(Config{GitHubSecret: ""})
	t.Setenv("GITHUB_WEBHOOK_SECRET", "")
	// Already constructed above — explicitly re-construct so env override
	// is what we test on the New() path.
	s, _ = New(Config{})
	req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
}

func TestGithubMissingSig_401(t *testing.T) {
	s, _ := New(Config{GitHubSecret: "topsecret"})
	req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestGithubMismatch_401(t *testing.T) {
	s, _ := New(Config{GitHubSecret: "topsecret"})
	body := []byte(`{"action":"opened"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", ghSig("wrong", body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestGithubValid_200(t *testing.T) {
	s, _ := New(Config{GitHubSecret: "topsecret"})
	body := []byte(`{"action":"opened","number":1}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/github", bytes.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", ghSig("topsecret", body))
	req.Header.Set("X-GitHub-Event", "issues")
	req.Header.Set("X-GitHub-Delivery", "test-deliv-001")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%q)", rec.Code, rec.Body.String())
	}
}

func TestGithubGet_405(t *testing.T) {
	s, _ := New(Config{GitHubSecret: "topsecret"})
	req := httptest.NewRequest(http.MethodGet, "/webhook/github", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}

func planeSig(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func TestPlaneNoSecret_503(t *testing.T) {
	t.Setenv("PLANE_WEBHOOK_SECRET", "")
	s, _ := New(Config{})
	req := httptest.NewRequest(http.MethodPost, "/webhook/plane", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}
}

func TestPlaneMissingSig_401(t *testing.T) {
	s, _ := New(Config{PlaneSecret: "topsecret"})
	req := httptest.NewRequest(http.MethodPost, "/webhook/plane", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestPlaneMismatch_401(t *testing.T) {
	s, _ := New(Config{PlaneSecret: "topsecret"})
	body := []byte(`{"event":"issue","action":"create"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/plane", bytes.NewReader(body))
	req.Header.Set("X-Plane-Signature", planeSig("wrong", body))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rec.Code)
	}
}

func TestPlaneValid_200(t *testing.T) {
	s, _ := New(Config{PlaneSecret: "topsecret"})
	body := []byte(`{"event":"issue","action":"create","workspace_slug":"frac","data":{"name":"hello"}}`)
	req := httptest.NewRequest(http.MethodPost, "/webhook/plane", bytes.NewReader(body))
	req.Header.Set("X-Plane-Signature", planeSig("topsecret", body))
	req.Header.Set("X-Plane-Delivery", "test-deliv-001")
	req.Header.Set("X-Plane-Event", "issue")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d (body=%q)", rec.Code, rec.Body.String())
	}
}

func TestPlaneGet_405(t *testing.T) {
	s, _ := New(Config{PlaneSecret: "x"})
	req := httptest.NewRequest(http.MethodGet, "/webhook/plane", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}
