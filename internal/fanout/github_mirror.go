package fanout

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/frac-labs/webhook-router/internal/normalize"
	"github.com/frac-labs/webhook-router/internal/subscribers"
)

// fireGitHubIssueMirror mirrors a Plane issue event into a GitHub issue.
// Direction: plane → github. Only fires when ev.Source == "plane" and
// ev.Kind == "issue".
//
// Loop prevention: if the inbound event body already carries our marker,
// the issue was created by our plane_issue_mirror forward path — skip.
func (d *Dispatcher) fireGitHubIssueMirror(ctx context.Context, s subscribers.Subscriber, ev normalize.Event) error {
	if ev.Source != "plane" || ev.Kind != "issue" {
		return nil
	}
	if strings.Contains(ev.Body, s.EffectiveMarkerPrefix()) {
		d.logger.Info("mirror loop-skip", "subscriber", s.Name, "direction", "plane->gh")
		return nil
	}
	if s.GitHubRepo == "" || s.GitHubTokenEnv == "" {
		return fmt.Errorf("github_issue_mirror %s: missing github_repo/github_token_env", s.Name)
	}
	token := os.Getenv(s.GitHubTokenEnv)
	if token == "" {
		return fmt.Errorf("github_issue_mirror %s: env %s empty", s.Name, s.GitHubTokenEnv)
	}

	// Marker identifies this Plane issue uniquely. ev.Title for Plane is
	// "PROJ-N: name" (see normalize/plane.go); we encode workspace + title-
	// prefix as the stable identifier.
	planeID := strings.SplitN(ev.Title, ":", 2)[0] // "PROJ-N"
	marker := fmt.Sprintf("%splane=%s/%s-->", s.EffectiveMarkerPrefix(), ev.Target, planeID)

	base := strings.TrimRight(s.URL, "/")
	if base == "" {
		base = "https://api.github.com"
	}

	existingNum, err := githubFindByMarker(ctx, d.client, base, token, s.GitHubRepo, marker)
	if err != nil {
		return fmt.Errorf("github search: %w", err)
	}

	body := buildMirrorDescription(ev, marker)
	if existingNum == 0 {
		createURL := fmt.Sprintf("%s/repos/%s/issues", base, s.GitHubRepo)
		return githubJSON(ctx, d.client, http.MethodPost, createURL, token, map[string]any{
			"title": ev.Title,
			"body":  body,
		})
	}
	patchURL := fmt.Sprintf("%s/repos/%s/issues/%d", base, s.GitHubRepo, existingNum)
	return githubJSON(ctx, d.client, http.MethodPatch, patchURL, token, map[string]any{
		"title": ev.Title,
		"body":  body,
	})
}

func githubFindByMarker(ctx context.Context, client *http.Client, base, token, repo, marker string) (int, error) {
	// GitHub search-issues API: q=<marker> in:body repo:owner/repo
	q := fmt.Sprintf("%s in:body repo:%s", marker, repo)
	searchURL := fmt.Sprintf("%s/search/issues?q=%s&per_page=5", base, url.QueryEscape(q))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("github search HTTP %d: %s", resp.StatusCode, string(b))
	}
	var out struct {
		Items []struct {
			Number int    `json:"number"`
			Body   string `json:"body"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	for _, it := range out.Items {
		if strings.Contains(it.Body, marker) {
			return it.Number, nil
		}
	}
	return 0, nil
}

func githubJSON(ctx context.Context, client *http.Client, method, urlStr, token string, payload any) error {
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, urlStr, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("github %s %s: HTTP %d: %s", method, urlStr, resp.StatusCode, string(body))
	}
	return nil
}
