package fanout

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/frac-labs/webhook-router/internal/normalize"
	"github.com/frac-labs/webhook-router/internal/subscribers"
)

// TestPlaneMirror_CreatesWhenNoMatch verifies the gh→plane forward path
// POSTs a new work item and appends the marker.
func TestPlaneMirror_CreatesWhenNoMatch(t *testing.T) {
	var posts atomic.Int32
	var gotPayload map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/work-items/search/"):
			// Empty result list: no existing mirror.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[]}`))
		case strings.Contains(r.URL.Path, "/work-items/") && r.Method == http.MethodPost:
			posts.Add(1)
			_ = json.NewDecoder(r.Body).Decode(&gotPayload)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Logf("unexpected path %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	t.Setenv("PLANE_PAT", "test-key")
	sub := subscribers.Subscriber{
		Name: "plane-mirror", Kind: "plane_issue_mirror",
		URL: srv.URL, Source: "github", Events: []string{"issues.*"},
		PlaneWorkspaceSlug: "ws", PlaneProjectID: "proj-uuid", PlaneAPIKeyEnv: "PLANE_PAT",
	}
	d := New([]subscribers.Subscriber{sub}, newLogger())
	ev := normalize.Event{
		Source: "github", Kind: "issues", Action: "opened",
		Actor: "alice", Target: "frac-labs/clawdiovascular",
		URL:   "https://github.com/frac-labs/clawdiovascular/issues/42",
		Title: "boom", Body: "details",
	}
	n := d.Dispatch(context.Background(), ev)
	if n != 1 {
		t.Fatalf("attempts = %d, want 1", n)
	}
	if posts.Load() != 1 {
		t.Fatalf("plane POST = %d, want 1", posts.Load())
	}
	desc, _ := gotPayload["description_html"].(string)
	if !strings.Contains(desc, "<!--webhook-router:mirror gh=frac-labs/clawdiovascular#42-->") {
		t.Fatalf("description missing marker: %q", desc)
	}
}

// TestPlaneMirror_PatchesWhenMatch verifies that an existing mirror is
// PATCHed rather than re-created.
func TestPlaneMirror_PatchesWhenMatch(t *testing.T) {
	var posts, patches atomic.Int32
	marker := "<!--webhook-router:mirror gh=frac-labs/x#7-->"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/work-items/search/"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"results":[{"id":"abc-123","description_html":"` + marker + `"}]}`))
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/work-items/"):
			posts.Add(1)
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPatch && strings.Contains(r.URL.Path, "/work-items/abc-123/"):
			patches.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			t.Logf("unexpected path %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	t.Setenv("PLANE_PAT", "test-key")
	sub := subscribers.Subscriber{
		Name: "plane-mirror", Kind: "plane_issue_mirror",
		URL: srv.URL, Source: "github", Events: []string{"issues.*"},
		PlaneWorkspaceSlug: "ws", PlaneProjectID: "proj-uuid", PlaneAPIKeyEnv: "PLANE_PAT",
	}
	d := New([]subscribers.Subscriber{sub}, newLogger())
	ev := normalize.Event{
		Source: "github", Kind: "issues", Action: "edited",
		Target: "frac-labs/x", URL: "https://github.com/frac-labs/x/issues/7",
		Title: "updated", Body: "fresh body",
	}
	d.Dispatch(context.Background(), ev)
	if patches.Load() != 1 || posts.Load() != 0 {
		t.Fatalf("expected 1 PATCH 0 POST, got patches=%d posts=%d", patches.Load(), posts.Load())
	}
}

// TestPlaneMirror_LoopSkip verifies an event whose body already carries our
// marker is silently skipped (no Plane API calls).
func TestPlaneMirror_LoopSkip(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("PLANE_PAT", "test-key")
	sub := subscribers.Subscriber{
		Name: "plane-mirror", Kind: "plane_issue_mirror",
		URL: srv.URL, Source: "github", Events: []string{"issues.*"},
		PlaneWorkspaceSlug: "ws", PlaneProjectID: "proj-uuid", PlaneAPIKeyEnv: "PLANE_PAT",
	}
	d := New([]subscribers.Subscriber{sub}, newLogger())
	ev := normalize.Event{
		Source: "github", Kind: "issues", Action: "edited",
		Target: "frac-labs/x", URL: "https://github.com/frac-labs/x/issues/9",
		Title: "echo", Body: "body\n\n<!--webhook-router:mirror plane=ws/PROJ-3-->",
	}
	d.Dispatch(context.Background(), ev)
	if calls.Load() != 0 {
		t.Fatalf("plane API was called %d times; loop-skip failed", calls.Load())
	}
}

// TestGitHubMirror_CreatesWhenNoMatch verifies plane→github forward POSTs.
func TestGitHubMirror_CreatesWhenNoMatch(t *testing.T) {
	var posts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/issues"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"items":[]}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/repos/"):
			posts.Add(1)
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "webhook-router:mirror plane=") {
				t.Errorf("create body missing marker: %s", string(body))
			}
			w.WriteHeader(http.StatusCreated)
		default:
			t.Logf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	t.Setenv("GH_TOKEN", "tok")
	sub := subscribers.Subscriber{
		Name: "gh-mirror", Kind: "github_issue_mirror",
		URL: srv.URL, Source: "plane", Events: []string{"issue.*"},
		GitHubRepo: "frac-labs/clawdiovascular", GitHubTokenEnv: "GH_TOKEN",
	}
	d := New([]subscribers.Subscriber{sub}, newLogger())
	ev := normalize.Event{
		Source: "plane", Kind: "issue", Action: "create",
		Target: "ws", Title: "PROJ-12: something", Body: "details",
	}
	d.Dispatch(context.Background(), ev)
	if posts.Load() != 1 {
		t.Fatalf("github POST = %d, want 1", posts.Load())
	}
}

// TestGitHubMirror_LoopSkip — symmetric loop-skip on plane→github direction.
func TestGitHubMirror_LoopSkip(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("GH_TOKEN", "tok")
	sub := subscribers.Subscriber{
		Name: "gh-mirror", Kind: "github_issue_mirror",
		URL: srv.URL, Source: "plane", Events: []string{"issue.*"},
		GitHubRepo: "frac-labs/x", GitHubTokenEnv: "GH_TOKEN",
	}
	d := New([]subscribers.Subscriber{sub}, newLogger())
	ev := normalize.Event{
		Source: "plane", Kind: "issue", Action: "update",
		Target: "ws", Title: "PROJ-1: x",
		Body: "stuff\n\n<!--webhook-router:mirror gh=frac-labs/x#5-->",
	}
	d.Dispatch(context.Background(), ev)
	if calls.Load() != 0 {
		t.Fatalf("github API called %d times; loop-skip failed", calls.Load())
	}
}

// TestSubscriber_MarkerPrefixDefault ensures the helper returns the package
// default when MarkerPrefix is empty.
func TestSubscriber_MarkerPrefixDefault(t *testing.T) {
	s := subscribers.Subscriber{}
	if got := s.EffectiveMarkerPrefix(); got != subscribers.DefaultMarkerPrefix {
		t.Fatalf("default = %q, want %q", got, subscribers.DefaultMarkerPrefix)
	}
	s.MarkerPrefix = "<!--custom "
	if got := s.EffectiveMarkerPrefix(); got != "<!--custom " {
		t.Fatalf("override = %q", got)
	}
}
