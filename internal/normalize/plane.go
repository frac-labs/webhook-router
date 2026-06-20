package normalize

import (
	"encoding/json"
	"fmt"
)

// Plane maps a Plane webhook JSON body into a canonical Event.
//
// Plane payload shape (per makeplane/plane apps/api/plane/bgtasks/webhook_task.py):
//
//	{
//	  "event": "<kind>",      // "issue", "project", "cycle", "module", ...
//	  "action": "<verb>",     // "create", "update", "delete"
//	  "webhook_id": "...",
//	  "workspace_id": "...",
//	  "workspace_slug": "...",
//	  "data": { ... },        // kind-specific
//	  "activity": { ... }
//	}
//
// Unknown event kinds still return a usable envelope; data fields are
// extracted best-effort from common shapes (issue, project).
func Plane(body []byte) (Event, error) {
	ev := Event{Source: "plane"}
	if len(body) == 0 {
		return ev, fmt.Errorf("plane: empty body")
	}

	var p struct {
		Event         string `json:"event"`
		Action        string `json:"action"`
		WorkspaceSlug string `json:"workspace_slug"`
		Data          struct {
			ID           string `json:"id"`
			Name         string `json:"name"`        // project, cycle, module, page
			Identifier   string `json:"identifier"`  // project key
			Description  string `json:"description"`
			SequenceID   int    `json:"sequence_id"` // issue number within project
			ProjectID    string `json:"project_id"`
			URL          string `json:"url"`         // some payloads carry this
			CreatedBy    string `json:"created_by"`
			UpdatedBy    string `json:"updated_by"`
		} `json:"data"`
		Activity struct {
			Actor struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
			} `json:"actor"`
		} `json:"activity"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return ev, err
	}

	ev.Kind = p.Event
	ev.Action = p.Action
	if p.Activity.Actor.DisplayName != "" {
		ev.Actor = p.Activity.Actor.DisplayName
	} else {
		ev.Actor = p.Activity.Actor.ID
	}
	ev.Target = p.WorkspaceSlug
	ev.URL = p.Data.URL

	switch p.Event {
	case "issue":
		ev.Title = p.Data.Name
		if p.Data.SequenceID > 0 && p.Data.Identifier != "" {
			ev.Title = fmt.Sprintf("%s-%d: %s", p.Data.Identifier, p.Data.SequenceID, p.Data.Name)
		}
		ev.Body = truncate(p.Data.Description, 500)
	case "project", "cycle", "module", "page":
		ev.Title = p.Data.Name
		ev.Body = truncate(p.Data.Description, 500)
	default:
		ev.Title = p.Data.Name
	}
	return ev, nil
}
