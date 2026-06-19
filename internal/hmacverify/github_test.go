package hmacverify

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func TestVerifyGitHub_Valid(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	if err := VerifyGitHub("topsecret", sign("topsecret", body), body); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestVerifyGitHub_Mismatch(t *testing.T) {
	body := []byte(`{"action":"opened"}`)
	err := VerifyGitHub("topsecret", sign("wrongsecret", body), body)
	if !errors.Is(err, ErrMismatch) {
		t.Fatalf("want ErrMismatch, got %v", err)
	}
}

func TestVerifyGitHub_MissingHeader(t *testing.T) {
	if err := VerifyGitHub("s", "", []byte("x")); !errors.Is(err, ErrMissingHeader) {
		t.Fatalf("want ErrMissingHeader, got %v", err)
	}
}

func TestVerifyGitHub_MalformedNoPrefix(t *testing.T) {
	if err := VerifyGitHub("s", "deadbeef", []byte("x")); !errors.Is(err, ErrMalformedHeader) {
		t.Fatalf("want ErrMalformedHeader, got %v", err)
	}
}

func TestVerifyGitHub_MalformedNonHex(t *testing.T) {
	if err := VerifyGitHub("s", "sha256=not-hex", []byte("x")); !errors.Is(err, ErrMalformedHeader) {
		t.Fatalf("want ErrMalformedHeader, got %v", err)
	}
}

func TestVerifyGitHub_BodyTamper(t *testing.T) {
	good := sign("topsecret", []byte(`{"action":"opened"}`))
	err := VerifyGitHub("topsecret", good, []byte(`{"action":"closed"}`))
	if !errors.Is(err, ErrMismatch) {
		t.Fatalf("want ErrMismatch, got %v", err)
	}
}
