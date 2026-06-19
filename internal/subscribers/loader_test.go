package subscribers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_Missing(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(cfg.Subscribers) != 0 {
		t.Fatalf("want empty config, got %d subs", len(cfg.Subscribers))
	}
}

func TestLoad_Valid(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "subs.yaml")
	if err := os.WriteFile(p, []byte(`subscribers:
  - name: mattermost-fractura
    kind: mattermost
    url: http://mattermost.example.svc:8065/hooks/abc
    events: ["issues.opened", "issues.closed"]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Subscribers) != 1 {
		t.Fatalf("want 1 sub, got %d", len(cfg.Subscribers))
	}
	s := cfg.Subscribers[0]
	if s.Name != "mattermost-fractura" || s.Kind != "mattermost" {
		t.Fatalf("unexpected sub: %+v", s)
	}
	if len(s.Events) != 2 {
		t.Fatalf("want 2 events, got %d", len(s.Events))
	}
}

func TestLoad_MissingFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(p, []byte(`subscribers:
  - name: x
    kind: mattermost
    url: ""
    events: ["a"]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected validate error for missing url")
	}
}

func TestMatch(t *testing.T) {
	s := Subscriber{Events: []string{"issues.*", "pull_request.opened"}}
	cases := map[string]bool{
		"issues.opened":         true,
		"issues.closed":         true,
		"pull_request.opened":   true,
		"pull_request.closed":   false,
		"push":                  false,
	}
	for ev, want := range cases {
		if got := s.Matches(ev); got != want {
			t.Errorf("Matches(%q) = %v, want %v", ev, got, want)
		}
	}
}
