package fanout

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/frac-labs/webhook-router/internal/normalize"
	"github.com/frac-labs/webhook-router/internal/subscribers"
)

func newLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestDispatch_MattermostHappy(t *testing.T) {
	var calls atomic.Int32
	var gotText string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var p mattermostPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			t.Errorf("decode: %v", err)
		}
		gotText = p.Text
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	subs := []subscribers.Subscriber{
		{Name: "mm", Kind: "mattermost", URL: srv.URL, Events: []string{"issues.*"}},
		{Name: "ignored", Kind: "mattermost", URL: srv.URL, Events: []string{"push"}},
	}
	d := New(subs, newLogger())
	ev := normalize.Event{Source: "github", Kind: "issues", Action: "opened",
		Actor: "alice", Target: "f/r", URL: "https://x", Title: "boom"}
	n := d.Dispatch(context.Background(), ev)
	if n != 1 {
		t.Fatalf("attempts = %d, want 1", n)
	}
	if calls.Load() != 1 {
		t.Fatalf("server calls = %d, want 1", calls.Load())
	}
	if gotText == "" {
		t.Fatal("empty text")
	}
}

func TestDispatch_Non2xxLogsButDoesntPanic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()
	d := New([]subscribers.Subscriber{
		{Name: "bad", Kind: "mattermost", URL: srv.URL, Events: []string{"*"}},
	}, newLogger())
	n := d.Dispatch(context.Background(), normalize.Event{Kind: "x"})
	if n != 1 {
		t.Fatalf("attempts = %d, want 1", n)
	}
}

func TestDispatch_UnsupportedKindCounted(t *testing.T) {
	d := New([]subscribers.Subscriber{
		{Name: "p", Kind: "plane", URL: "http://x", Events: []string{"*"}},
	}, newLogger())
	n := d.Dispatch(context.Background(), normalize.Event{Kind: "x"})
	if n != 1 {
		t.Fatalf("attempts = %d, want 1", n)
	}
}

func TestFormatMattermost_Fallbacks(t *testing.T) {
	cases := []normalize.Event{
		{Kind: "issues", Action: "opened", URL: "u", Title: "t", Actor: "a", Target: "r"},
		{Kind: "push", URL: "u", Actor: "a", Target: "r"},
		{Kind: "ping", Actor: "a", Target: "r"},
	}
	for _, ev := range cases {
		if formatMattermost(ev) == "" {
			t.Errorf("empty format for %+v", ev)
		}
	}
}

func TestMain(m *testing.M) {
	// Quiet the default logger during tests.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	os.Exit(m.Run())
}
