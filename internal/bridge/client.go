// Package bridge is a stub harness-bridge client.
//
// v0.1.0 has no functional bridge integration. Real client lands when
// harness-protos v0.2.x adds the webhook-router-relevant RPCs (or we wire
// MintGitHubToken for github-app-token-needing fan-out targets).
package bridge

// Client is a placeholder for the harness-bridge gRPC client.
type Client struct{}

// New returns a stub client. addr/ca/cert/key are accepted for forward-compat
// but unused in v0.1.0.
func New(addr, caPath, certPath, keyPath string) (*Client, error) {
	return &Client{}, nil
}
