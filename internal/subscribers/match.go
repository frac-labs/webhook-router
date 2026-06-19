package subscribers

import "path/filepath"

// Matches returns true if event matches any of the subscriber's event globs.
// Globs use filepath.Match semantics (* matches any non-separator chars).
// Event names are dotted: "issues.opened", "pull_request.closed".
func (s Subscriber) Matches(event string) bool {
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
