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

// firePlaneIssueMirror mirrors a GitHub issue event into a Plane work item.
// Direction: github → plane. Only fires when ev.Source == "github" and
// ev.Kind == "issues". Lookup-by-marker; create or update accordingly.
//
// Loop prevention: if the inbound event body already carries our marker,
// the issue was created by our github_issue_mirror reverse path — skip.
func (d *Dispatcher) firePlaneIssueMirror(ctx context.Context, s subscribers.Subscriber, ev normalize.Event) error {
	if ev.Source != "github" || ev.Kind != "issues" {
		return nil // silent no-op; matcher should usually filter, defensive guard.
	}
	if strings.Contains(ev.Body, s.EffectiveMarkerPrefix()) {
		d.logger.Info("mirror loop-skip", "subscriber", s.Name, "direction", "gh->plane")
		return nil
	}
	if s.PlaneWorkspaceSlug == "" || s.PlaneProjectID == "" || s.PlaneAPIKeyEnv == "" {
		return fmt.Errorf("plane_issue_mirror %s: missing plane_workspace_slug/plane_project_id/plane_api_key_env", s.Name)
	}
	apiKey := os.Getenv(s.PlaneAPIKeyEnv)
	if apiKey == "" {
		return fmt.Errorf("plane_issue_mirror %s: env %s empty", s.Name, s.PlaneAPIKeyEnv)
	}

	// Marker identifies this GitHub issue uniquely.
	marker := fmt.Sprintf("%sgh=%s#%s-->", s.EffectiveMarkerPrefix(), ev.Target, extractIssueNumber(ev.URL))

	base := strings.TrimRight(s.URL, "/")
	listURL := fmt.Sprintf("%s/api/v1/workspaces/%s/work-items/search/?search=%s&project_id=%s&limit=5",
		base, s.PlaneWorkspaceSlug, url.QueryEscape(marker), s.PlaneProjectID)

	existingID, err := planeFindByMarker(ctx, d.client, listURL, apiKey, marker)
	if err != nil {
		return fmt.Errorf("plane search: %w", err)
	}

	descHTML := buildMirrorDescription(ev, marker)
	payload := map[string]any{
		"name":             ev.Title,
		"description_html": descHTML,
	}

	if existingID != "" {
		patchURL := fmt.Sprintf("%s/api/v1/workspaces/%s/projects/%s/work-items/%s/",
			base, s.PlaneWorkspaceSlug, s.PlaneProjectID, existingID)
		return planeJSON(ctx, d.client, http.MethodPatch, patchURL, apiKey, payload)
	}
	createURL := fmt.Sprintf("%s/api/v1/workspaces/%s/projects/%s/work-items/",
		base, s.PlaneWorkspaceSlug, s.PlaneProjectID)
	return planeJSON(ctx, d.client, http.MethodPost, createURL, apiKey, payload)
}

// extractIssueNumber pulls the trailing "N" out of a GitHub issue HTML URL.
// On parse failure returns "0" — the marker still works as a stable string
// for that ev.URL on the next round; downstream search is exact-substring.
func extractIssueNumber(htmlURL string) string {
	if htmlURL == "" {
		return "0"
	}
	parts := strings.Split(htmlURL, "/")
	if len(parts) == 0 {
		return "0"
	}
	last := parts[len(parts)-1]
	if last == "" {
		return "0"
	}
	return last
}

func buildMirrorDescription(ev normalize.Event, marker string) string {
	var b strings.Builder
	if ev.Body != "" {
		b.WriteString(ev.Body)
		b.WriteString("\n\n")
	}
	if ev.URL != "" {
		fmt.Fprintf(&b, "Mirrored from: %s\n\n", ev.URL)
	}
	b.WriteString(marker)
	return b.String()
}

// planeFindByMarker calls Plane's work-item search endpoint and returns the
// first result whose description_html contains the marker. Returns "" if no
// match. The search endpoint already filters by substring; we double-check
// here because Plane's search may match other fields.
func planeFindByMarker(ctx context.Context, client *http.Client, listURL, apiKey, marker string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("plane search HTTP %d: %s", resp.StatusCode, string(body))
	}
	// Search response shape varies between Plane versions; we accept either a
	// bare list or an object with "results". Each item should carry id +
	// description / description_html.
	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return "", err
	}
	var asObj struct {
		Results []planeSearchHit `json:"results"`
	}
	if err := json.Unmarshal(raw, &asObj); err == nil && asObj.Results != nil {
		return firstHitWithMarker(asObj.Results, marker), nil
	}
	var asArr []planeSearchHit
	if err := json.Unmarshal(raw, &asArr); err == nil {
		return firstHitWithMarker(asArr, marker), nil
	}
	return "", nil
}

type planeSearchHit struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	DescriptionHTML string `json:"description_html"`
}

func firstHitWithMarker(hits []planeSearchHit, marker string) string {
	for _, h := range hits {
		if strings.Contains(h.DescriptionHTML, marker) || strings.Contains(h.Description, marker) {
			return h.ID
		}
	}
	return ""
}

func planeJSON(ctx context.Context, client *http.Client, method, url, apiKey string, payload any) error {
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("plane %s %s: HTTP %d: %s", method, url, resp.StatusCode, string(body))
	}
	return nil
}
