package subscribers

import "path/filepath"

// Matches returns true if event matches any of the subscriber's event globs
// AND the subscriber's Source filter accepts the given event source. An empty
// Source on the subscriber means "match-any source" (back-compat with configs
// authored before the Source field existed).
//
// Globs use filepath.Match semantics (* matches any non-separator chars).
// Event names are dotted: "issues.opened", "pull_request.closed",
// "issue.created", "project.update". The source is the canonical normalize
// envelope source: "github", "plane", etc.
func (s Subscriber) Matches(source, event string) bool {
	if s.Source != "" && s.Source != source {
		return false
	}
	for _, pat := range s.Events {
		if pat == event {
			return true
		}
		ok, err := filepath.Match(pat, event)
		if err == nil && ok {
			return true
		}
	}
	return false
}
