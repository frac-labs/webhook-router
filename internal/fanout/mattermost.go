// Package fanout dispatches normalized webhook events to subscriber targets.
//
// PR-4b implements the Mattermost-style incoming-webhook target (slack-
// compatible JSON, "text" field). Other kinds (plane, hermes, frac) are
// stubs returning ErrUnsupported.
package fanout

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/frac-labs/webhook-router/internal/normalize"
	"github.com/frac-labs/webhook-router/internal/subscribers"
)

// ErrUnsupported is returned by Dispatch when the subscriber kind has no
// handler wired yet.
var ErrUnsupported = errors.New("fanout: unsupported subscriber kind")

// DefaultTimeout is the per-target HTTP call timeout.
const DefaultTimeout = 5 * time.Second

// Dispatcher fans events out to matching subscribers concurrently.
type Dispatcher struct {
	subs   []subscribers.Subscriber
	logger *slog.Logger
	client *http.Client
}

// New returns a Dispatcher over the given subscribers.
func New(subs []subscribers.Subscriber, logger *slog.Logger) *Dispatcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{
		subs:   subs,
		logger: logger,
		client: &http.Client{Timeout: DefaultTimeout},
	}
}

// Dispatch evaluates ev against every subscriber and fires concurrently to
// matches. Returns the number of targets attempted. Per-target errors are
// logged, not returned (one bad subscriber must not break the others).
func (d *Dispatcher) Dispatch(ctx context.Context, ev normalize.Event) int {
	name := ev.EventName()
	var wg sync.WaitGroup
	attempts := 0
	for _, s := range d.subs {
		if !s.Matches(ev.Source, name) {
			continue
		}
		attempts++
		wg.Add(1)
		go func(sub subscribers.Subscriber) {
			defer wg.Done()
			if err := d.fire(ctx, sub, ev); err != nil {
				d.logger.Warn("fanout failed",
					"subscriber", sub.Name, "kind", sub.Kind,
					"event", name, "err", err)
				return
			}
			d.logger.Info("fanout ok",
				"subscriber", sub.Name, "kind", sub.Kind, "event", name)
		}(s)
	}
	wg.Wait()
	return attempts
}

func (d *Dispatcher) fire(ctx context.Context, s subscribers.Subscriber, ev normalize.Event) error {
	switch s.Kind {
	case "mattermost":
		return d.fireMattermost(ctx, s, ev)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupported, s.Kind)
	}
}

// mattermostPayload is the slack-compatible incoming-webhook body shape.
// Mattermost incoming webhooks accept Slack JSON; we use only "text".
type mattermostPayload struct {
	Text     string `json:"text"`
	Username string `json:"username,omitempty"`
}

func (d *Dispatcher) fireMattermost(ctx context.Context, s subscribers.Subscriber, ev normalize.Event) error {
	payload := mattermostPayload{
		Text:     formatMattermost(ev),
		Username: "webhook-router",
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.URL, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("mattermost %s: HTTP %d: %s", s.URL, resp.StatusCode, string(body))
	}
	return nil
}

// formatMattermost renders the envelope into a single-line markdown message.
func formatMattermost(ev normalize.Event) string {
	name := ev.EventName()
	switch {
	case ev.URL != "" && ev.Title != "":
		return fmt.Sprintf("**[%s]** %s by `%s` in `%s`: [%s](%s)",
			name, capActor(ev.Actor), ev.Actor, ev.Target, ev.Title, ev.URL)
	case ev.URL != "":
		return fmt.Sprintf("**[%s]** by `%s` in `%s`: %s", name, ev.Actor, ev.Target, ev.URL)
	default:
		return fmt.Sprintf("**[%s]** by `%s` in `%s`", name, ev.Actor, ev.Target)
	}
}

func capActor(a string) string {
	if a == "" {
		return "(unknown)"
	}
	return a
}
