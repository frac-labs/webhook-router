// Package subscribers loads the subscribers.yaml config at startup.
//
// PR-4b adds the read-once loader. fsnotify-style hot-reload is overkill at
// v0.2; pod restarts pick up config changes (ConfigMap roll → kubelet remount).
package subscribers

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load reads and parses the subscribers config at the given path.
// An empty/missing file is NOT an error — it returns an empty Config so the
// router runs as a verifier with no fan-out targets (loud-fail at fan-out
// time is more useful than refusing to start).
func Load(path string) (Config, error) {
	if path == "" {
		return Config{}, nil
	}
	clean := filepath.Clean(path)
	b, err := os.ReadFile(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return Config{}, nil
		}
		return Config{}, fmt.Errorf("read %s: %w", clean, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", clean, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate %s: %w", clean, err)
	}
	return cfg, nil
}

// Validate sanity-checks the loaded config.
func (c Config) Validate() error {
	for i, s := range c.Subscribers {
		if s.Name == "" {
			return fmt.Errorf("subscriber[%d]: name required", i)
		}
		if s.Kind == "" {
			return fmt.Errorf("subscriber[%s]: kind required", s.Name)
		}
		switch s.Kind {
		case "plane_issue_mirror":
			hasSlug := s.PlaneWorkspaceSlug != "" || s.PlaneWorkspaceSlugEnv != ""
			hasProj := s.PlaneProjectID != "" || s.PlaneProjectIDEnv != ""
			if !hasSlug || !hasProj || s.PlaneAPIKeyEnv == "" {
				return fmt.Errorf("subscriber[%s]: plane_workspace_slug(_env), plane_project_id(_env), and plane_api_key_env all required for plane_issue_mirror", s.Name)
			}
		case "github_issue_mirror":
			if s.GitHubRepo == "" || s.GitHubTokenEnv == "" {
				return fmt.Errorf("subscriber[%s]: github_repo and github_token_env both required for github_issue_mirror", s.Name)
			}
		default:
			if s.URL == "" {
				return fmt.Errorf("subscriber[%s]: url required", s.Name)
			}
		}
		if len(s.Events) == 0 {
			return fmt.Errorf("subscriber[%s]: at least one event glob required", s.Name)
		}
	}
	return nil
}
