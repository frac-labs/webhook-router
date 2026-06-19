// Package normalize maps webhook-source-specific payloads into a canonical
// {source, kind, action, actor, target, url} envelope that downstream
// fan-out targets can format uniformly.
package normalize

import (
	"encoding/json"
)

// Event is the canonical webhook envelope.
type Event struct {
	Source string `json:"source"` // e.g. "github"
	Kind   string `json:"kind"`   // e.g. "issues", "pull_request", "push"
	Action string `json:"action"` // e.g. "opened", "closed"; "" if no action
	Actor  string `json:"actor"`  // login of user who triggered (best-effort)
	Target string `json:"target"` // repo full-name, e.g. "frac-labs/clawdiovascular"
	URL    string `json:"url"`    // html_url for the triggering object
	Title  string `json:"title"`  // human-readable headline
	Body   string `json:"body"`   // optional descriptive body (truncated)
}

// EventName returns "kind.action" if both set, else "kind".
func (e Event) EventName() string {
	if e.Action != "" {
		return e.Kind + "." + e.Action
	}
	return e.Kind
}

// GitHub maps an X-GitHub-Event header value and JSON body into an Event.
// Unknown event kinds still return a usable envelope (kind set, other fields
// populated best-effort from common payload shapes).
func GitHub(eventHeader string, body []byte) (Event, error) {
	ev := Event{Source: "github", Kind: eventHeader}

	// Minimal common shape — we only deserialize the fields we need.
	var p struct {
		Action     string `json:"action"`
		Sender     struct{ Login string } `json:"sender"`
		Repository struct {
			FullName string `json:"full_name"`
			HTMLURL  string `json:"html_url"`
		} `json:"repository"`
		Issue struct {
			HTMLURL string `json:"html_url"`
			Title   string `json:"title"`
			Body    string `json:"body"`
			Number  int    `json:"number"`
		} `json:"issue"`
		PullRequest struct {
			HTMLURL string `json:"html_url"`
			Title   string `json:"title"`
			Body    string `json:"body"`
			Number  int    `json:"number"`
		} `json:"pull_request"`
		Comment struct {
			HTMLURL string `json:"html_url"`
			Body    string `json:"body"`
		} `json:"comment"`
		Ref     string `json:"ref"`
		Compare string `json:"compare"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &p); err != nil {
			return ev, err
		}
	}
	ev.Action = p.Action
	ev.Actor = p.Sender.Login
	ev.Target = p.Repository.FullName

	switch eventHeader {
	case "issues":
		ev.URL = p.Issue.HTMLURL
		ev.Title = p.Issue.Title
		ev.Body = truncate(p.Issue.Body, 500)
	case "pull_request":
		ev.URL = p.PullRequest.HTMLURL
		ev.Title = p.PullRequest.Title
		ev.Body = truncate(p.PullRequest.Body, 500)
	case "issue_comment":
		ev.URL = p.Comment.HTMLURL
		ev.Title = p.Issue.Title
		ev.Body = truncate(p.Comment.Body, 500)
	case "push":
		ev.URL = p.Compare
		ev.Title = p.Ref
	default:
		ev.URL = p.Repository.HTMLURL
	}
	return ev, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
