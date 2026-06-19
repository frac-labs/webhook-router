// Package hmacverify implements HMAC signature verification for webhook
// sources. v0.2.0 (PR-4a): GitHub only.
//
// GitHub signs webhook bodies with HMAC-SHA256 keyed on a per-app shared
// secret and ships the digest in the `X-Hub-Signature-256` header as
// `sha256=<hex>`. We verify constant-time and reject on mismatch.
//
// Reference: https://docs.github.com/en/webhooks/using-webhooks/validating-webhook-deliveries
package hmacverify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// ErrMissingHeader is returned when the request has no signature header.
var ErrMissingHeader = errors.New("hmacverify: missing signature header")

// ErrMalformedHeader is returned when the signature header is malformed.
var ErrMalformedHeader = errors.New("hmacverify: malformed signature header")

// ErrMismatch is returned when the signature does not match the body.
var ErrMismatch = errors.New("hmacverify: signature mismatch")

// VerifyGitHub checks a GitHub `X-Hub-Signature-256` header value against
// the given body using the provided secret. Returns nil on a valid signature.
//
// The header value is the full `sha256=<hex>` form GitHub sends.
func VerifyGitHub(secret, headerVal string, body []byte) error {
	if headerVal == "" {
		return ErrMissingHeader
	}
	const prefix = "sha256="
	if !strings.HasPrefix(headerVal, prefix) {
		return ErrMalformedHeader
	}
	gotHex := headerVal[len(prefix):]
	got, err := hex.DecodeString(gotHex)
	if err != nil {
		return ErrMalformedHeader
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	want := mac.Sum(nil)
	if !hmac.Equal(got, want) {
		return ErrMismatch
	}
	return nil
}
