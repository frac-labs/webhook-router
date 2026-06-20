package hmacverify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func planeSig(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

func TestVerifyPlane_OK(t *testing.T) {
	body := []byte(`{"event":"issue","action":"create"}`)
	if err := VerifyPlane("sekret", planeSig("sekret", body), body); err != nil {
		t.Fatalf("want nil, got %v", err)
	}
}

func TestVerifyPlane_Missing(t *testing.T) {
	if err := VerifyPlane("sekret", "", []byte("x")); !errors.Is(err, ErrMissingHeader) {
		t.Fatalf("want ErrMissingHeader, got %v", err)
	}
}

func TestVerifyPlane_Malformed(t *testing.T) {
	if err := VerifyPlane("sekret", "not-hex-zz", []byte("x")); !errors.Is(err, ErrMalformedHeader) {
		t.Fatalf("want ErrMalformedHeader, got %v", err)
	}
}

func TestVerifyPlane_Mismatch(t *testing.T) {
	body := []byte(`{"event":"issue"}`)
	if err := VerifyPlane("sekret", planeSig("wrong", body), body); !errors.Is(err, ErrMismatch) {
		t.Fatalf("want ErrMismatch, got %v", err)
	}
}

func TestVerifyPlane_NoPrefix(t *testing.T) {
	// Distinguishing characteristic vs GitHub: no "sha256=" prefix accepted.
	body := []byte(`{}`)
	bad := "sha256=" + planeSig("sekret", body)
	if err := VerifyPlane("sekret", bad, body); !errors.Is(err, ErrMalformedHeader) {
		t.Fatalf("want ErrMalformedHeader, got %v", err)
	}
}
