package webhookdelivery

import (
	"errors"
	"testing"
	"time"
)

func TestHMACSignatureAndPayloadTampering(t *testing.T) {
	secret := "production-hmac-key-999"
	payload := []byte(`{"event":"order.placed","amount":150}`)

	now := time.Now().UTC()
	sig, _ := SignPayload(payload, now, secret)

	// 1. Valid verification
	err := VerifySignature(payload, sig, secret, 5*time.Minute)
	if err != nil {
		t.Fatalf("expected valid signature, got error: %v", err)
	}

	// 2. Tampered payload
	tamperedPayload := []byte(`{"event":"order.placed","amount":999}`)
	errTampered := VerifySignature(tamperedPayload, sig, secret, 5*time.Minute)
	if !errors.Is(errTampered, ErrSignatureMismatch) {
		t.Fatalf("expected ErrSignatureMismatch for tampered payload, got %v", errTampered)
	}

	// 3. Wrong secret
	errWrongSecret := VerifySignature(payload, sig, "different-wrong-secret", 5*time.Minute)
	if !errors.Is(errWrongSecret, ErrSignatureMismatch) {
		t.Fatalf("expected ErrSignatureMismatch for wrong secret, got %v", errWrongSecret)
	}
}

func TestAntiReplayTimestampWindow(t *testing.T) {
	secret := "test-secret"
	payload := []byte(`{"data":"secure"}`)

	// Timestamp 10 minutes in the past
	tenMinutesAgo := time.Now().UTC().Add(-10 * time.Minute)
	oldSig, _ := SignPayload(payload, tenMinutesAgo, secret)

	// Tolerance is 5 minutes -> should be rejected as expired
	err := VerifySignature(payload, oldSig, secret, 5*time.Minute)
	if !errors.Is(err, ErrReplayTimestampExpired) {
		t.Fatalf("expected ErrReplayTimestampExpired, got %v", err)
	}

	// Timestamp 2 minutes in the past (within 5-min tolerance) -> should succeed
	twoMinutesAgo := time.Now().UTC().Add(-2 * time.Minute)
	validSig, _ := SignPayload(payload, twoMinutesAgo, secret)
	if err := VerifySignature(payload, validSig, secret, 5*time.Minute); err != nil {
		t.Fatalf("expected valid signature within window, got %v", err)
	}
}

func TestMalformedSignatureHeaders(t *testing.T) {
	payload := []byte("hello")
	secret := "secret"

	malformedCases := []string{
		"",
		"garbage",
		"t=12345",
		"v1=abcdef",
		"t=abc,v1=123",
	}

	for _, tc := range malformedCases {
		err := VerifySignature(payload, tc, secret, 5*time.Minute)
		if err == nil {
			t.Errorf("expected error for malformed header %q", tc)
		}
	}
}
