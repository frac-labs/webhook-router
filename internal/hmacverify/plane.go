// Plane HMAC verification. v0.3.0 (PR-4c).
//
// Plane signs webhook bodies with HMAC-SHA256 keyed on a per-webhook shared
// secret. Unlike GitHub, Plane sends the raw hex digest in the
// `X-Plane-Signature` header with NO algorithm prefix.
//
// Reference: makeplane/plane apps/api/plane/bgtasks/webhook_task.py
//   hmac_signature = hmac.new(secret, json.dumps(payload), hashlib.sha256)
//   headers["X-Plane-Signature"] = hmac_signature.hexdigest()
package hmacverify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// VerifyPlane checks a Plane `X-Plane-Signature` header value (raw hex
// SHA256, no prefix) against the given body using the provided secret.
// Returns nil on a valid signature.
func VerifyPlane(secret, headerVal string, body []byte) error {
	if headerVal == "" {
		return ErrMissingHeader
	}
	got, err := hex.DecodeString(headerVal)
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
