// Package subscribers parses the subscribers.yaml config that maps webhook
// events to fan-out targets.
//
// v0.1.0 defines the shape but does not yet load or use it at runtime.
package subscribers

// Subscriber describes one fan-out target.
type Subscriber struct {
	Name   string   `yaml:"name"`
	Kind   string   `yaml:"kind"`   // mattermost | plane_issue_mirror | github_issue_mirror | hermes | frac
	URL    string   `yaml:"url"`    // target endpoint (or in-cluster svc); for mirror kinds: API base URL
	Source string   `yaml:"source"` // optional event-source filter (e.g. "github", "plane"); empty = match-any (back-compat)
	Events []string `yaml:"events"` // event-name glob list (filepath.Match against dotted EventName)

	// PR-1 (cdv#16): issue-mirror fields. All omitempty for back-compat.
	PlaneWorkspaceSlug    string `yaml:"plane_workspace_slug,omitempty"`
	PlaneWorkspaceSlugEnv string `yaml:"plane_workspace_slug_env,omitempty"` // env var holding workspace slug (alt to literal)
	PlaneProjectID        string `yaml:"plane_project_id,omitempty"`
	PlaneProjectIDEnv     string `yaml:"plane_project_id_env,omitempty"` // env var holding project id (alt to literal)
	PlaneAPIKeyEnv        string `yaml:"plane_api_key_env,omitempty"`     // env var holding X-API-Key
	GitHubRepo         string `yaml:"github_repo,omitempty"`       // owner/repo
	GitHubTokenEnv     string `yaml:"github_token_env,omitempty"`  // env var holding GitHub token
	MarkerPrefix       string `yaml:"marker_prefix,omitempty"`     // default "<!--webhook-router:mirror "
}

// DefaultMarkerPrefix is appended to mirrored issue descriptions/bodies as a
// loop-prevention + reverse-lookup key. Plane's work-item search hits
// description text, so the marker doubles as a discovery key.
const DefaultMarkerPrefix = "<!--webhook-router:mirror "

// EffectiveMarkerPrefix returns the configured marker prefix or the default.
func (s Subscriber) EffectiveMarkerPrefix() string {
	if s.MarkerPrefix != "" {
		return s.MarkerPrefix
	}
	return DefaultMarkerPrefix
}

// ResolvedPlaneWorkspaceSlug returns the literal value if set, else the value of
// the configured env var. Empty string if neither resolves.
func (s Subscriber) ResolvedPlaneWorkspaceSlug(getenv func(string) string) string {
	if s.PlaneWorkspaceSlug != "" {
		return s.PlaneWorkspaceSlug
	}
	if s.PlaneWorkspaceSlugEnv != "" {
		return getenv(s.PlaneWorkspaceSlugEnv)
	}
	return ""
}

// ResolvedPlaneProjectID returns the literal value if set, else the value of
// the configured env var. Empty string if neither resolves.
func (s Subscriber) ResolvedPlaneProjectID(getenv func(string) string) string {
	if s.PlaneProjectID != "" {
		return s.PlaneProjectID
	}
	if s.PlaneProjectIDEnv != "" {
		return getenv(s.PlaneProjectIDEnv)
	}
	return ""
}

// Config is the top-level subscribers.yaml shape.
type Config struct {
	Subscribers []Subscriber `yaml:"subscribers"`
}
