package normalize

import "testing"

func TestGitHub_Issues(t *testing.T) {
	body := []byte(`{
		"action": "opened",
		"sender": {"login": "alice"},
		"repository": {"full_name": "frac-labs/clawdiovascular", "html_url": "https://github.com/frac-labs/clawdiovascular"},
		"issue": {"html_url": "https://github.com/frac-labs/clawdiovascular/issues/42", "title": "boom", "body": "details here", "number": 42}
	}`)
	ev, err := GitHub("issues", body)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if ev.Source != "github" || ev.Kind != "issues" || ev.Action != "opened" {
		t.Fatalf("unexpected envelope: %+v", ev)
	}
	if ev.Actor != "alice" || ev.Target != "frac-labs/clawdiovascular" {
		t.Fatalf("actor/target: %+v", ev)
	}
	if ev.Title != "boom" || ev.URL == "" {
		t.Fatalf("title/url: %+v", ev)
	}
	if ev.EventName() != "issues.opened" {
		t.Fatalf("EventName: %s", ev.EventName())
	}
}

func TestGitHub_PullRequest(t *testing.T) {
	body := []byte(`{"action": "closed", "pull_request": {"html_url": "u", "title": "t"}}`)
	ev, _ := GitHub("pull_request", body)
	if ev.Action != "closed" || ev.Title != "t" || ev.URL != "u" {
		t.Fatalf("pr envelope: %+v", ev)
	}
}

func TestGitHub_UnknownKind(t *testing.T) {
	ev, err := GitHub("star", []byte(`{"action":"created","sender":{"login":"bob"}}`))
	if err != nil {
		t.Fatalf("unknown event should not error: %v", err)
	}
	if ev.Kind != "star" || ev.Action != "created" || ev.Actor != "bob" {
		t.Fatalf("unknown: %+v", ev)
	}
}

func TestGitHub_BadJSON(t *testing.T) {
	if _, err := GitHub("issues", []byte("not json")); err == nil {
		t.Fatal("expected error on bad json")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abc", 10); got != "abc" {
		t.Fatalf("short: %q", got)
	}
	if got := truncate("abcdefghij", 3); got != "abc…" {
		t.Fatalf("long: %q", got)
	}
}
