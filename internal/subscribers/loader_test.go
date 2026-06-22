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

func TestLoad_PlaneIssueMirror_NoURL(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "plane.yaml")
	if err := os.WriteFile(p, []byte(`subscribers:
  - name: gh-to-plane-mirror
    kind: plane_issue_mirror
    events: ["issues.opened"]
    source: github
    plane_workspace_slug: frac-labs
    plane_project_id: 00000000-0000-0000-0000-000000000000
    plane_api_key_env: PLANE_API_KEY
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("plane_issue_mirror without URL must load: %v", err)
	}
	if len(cfg.Subscribers) != 1 || cfg.Subscribers[0].Kind != "plane_issue_mirror" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestLoad_PlaneIssueMirror_MissingFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "plane-bad.yaml")
	if err := os.WriteFile(p, []byte(`subscribers:
  - name: gh-to-plane-mirror
    kind: plane_issue_mirror
    events: ["issues.opened"]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected validate error for missing plane fields")
	}
}

func TestLoad_GitHubIssueMirror_NoURL(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gh.yaml")
	if err := os.WriteFile(p, []byte(`subscribers:
  - name: plane-to-gh-mirror
    kind: github_issue_mirror
    events: ["issue.created"]
    source: plane
    github_repo: frac-labs/clawdiovascular
    github_token_env: GH_TOKEN
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("github_issue_mirror without URL must load: %v", err)
	}
	if len(cfg.Subscribers) != 1 || cfg.Subscribers[0].Kind != "github_issue_mirror" {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestLoad_GitHubIssueMirror_MissingFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "gh-bad.yaml")
	if err := os.WriteFile(p, []byte(`subscribers:
  - name: plane-to-gh-mirror
    kind: github_issue_mirror
    events: ["issue.created"]
    github_repo: frac-labs/clawdiovascular
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected validate error for missing github_token_env")
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
		// empty Source on subscriber = match-any (back-compat)
		if got := s.Matches("github", ev); got != want {
			t.Errorf("Matches(\"github\", %q) = %v, want %v", ev, got, want)
		}
	}
}

func TestLoad_PlaneIssueMirror_EnvVarFields(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "plane-env.yaml")
	if err := os.WriteFile(p, []byte(`subscribers:
  - name: gh-to-plane-mirror
    kind: plane_issue_mirror
    events: ["issues.opened"]
    source: github
    plane_workspace_slug_env: PLANE_WORKSPACE_SLUG
    plane_project_id_env: PLANE_PROJECT_ID
    plane_api_key_env: PLANE_PAT
`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(p)
	if err != nil {
		t.Fatalf("plane_issue_mirror with _env fields must load: %v", err)
	}
	if len(cfg.Subscribers) != 1 {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	s := cfg.Subscribers[0]
	if s.PlaneWorkspaceSlugEnv != "PLANE_WORKSPACE_SLUG" || s.PlaneProjectIDEnv != "PLANE_PROJECT_ID" {
		t.Fatalf("env fields not parsed: %+v", s)
	}
	getenv := func(k string) string {
		return map[string]string{"PLANE_WORKSPACE_SLUG": "frac-labs", "PLANE_PROJECT_ID": "proj-uuid"}[k]
	}
	if got := s.ResolvedPlaneWorkspaceSlug(getenv); got != "frac-labs" {
		t.Errorf("ResolvedPlaneWorkspaceSlug = %q, want frac-labs", got)
	}
	if got := s.ResolvedPlaneProjectID(getenv); got != "proj-uuid" {
		t.Errorf("ResolvedPlaneProjectID = %q, want proj-uuid", got)
	}
}

func TestMatchSourceFilter(t *testing.T) {
	gh := Subscriber{Source: "github", Events: []string{"*"}}
	pl := Subscriber{Source: "plane", Events: []string{"*"}}
	any := Subscriber{Events: []string{"*"}} // back-compat: no Source

	if !gh.Matches("github", "push") {
		t.Error("github subscriber should match github event")
	}
	if gh.Matches("plane", "issue.created") {
		t.Error("github subscriber MUST NOT match plane event")
	}
	if !pl.Matches("plane", "issue.created") {
		t.Error("plane subscriber should match plane event")
	}
	if pl.Matches("github", "push") {
		t.Error("plane subscriber MUST NOT match github event")
	}
	if !any.Matches("github", "push") || !any.Matches("plane", "issue.created") {
		t.Error("empty-Source subscriber must match any source (back-compat)")
	}
}
