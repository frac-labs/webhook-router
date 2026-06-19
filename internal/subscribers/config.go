// Package subscribers parses the subscribers.yaml config that maps webhook
// events to fan-out targets.
//
// v0.1.0 defines the shape but does not yet load or use it at runtime.
package subscribers

// Subscriber describes one fan-out target.
type Subscriber struct {
	Name   string   `yaml:"name"`
	Kind   string   `yaml:"kind"`   // mattermost | plane | hermes | frac
	URL    string   `yaml:"url"`    // target endpoint (or in-cluster svc)
	Events []string `yaml:"events"` // event-name glob list
}

// Config is the top-level subscribers.yaml shape.
type Config struct {
	Subscribers []Subscriber `yaml:"subscribers"`
}
