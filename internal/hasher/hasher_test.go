package hasher

import (
	"testing"
)

func TestHash_SHA256(t *testing.T) {
	result := Hash("hello", SHA256)
	if result == "" {
		t.Error("expected non-empty hash")
	}
	if len(result) != 64 {
		t.Errorf("expected 64 char hex, got %d", len(result))
	}
}

func TestHash_SHA512(t *testing.T) {
	result := Hash("hello", SHA512)
	if len(result) != 128 {
		t.Errorf("expected 128 char hex, got %d", len(result))
	}
}

func TestHash_Deterministic(t *testing.T) {
	a := Hash("test", SHA256)
	b := Hash("test", SHA256)
	if a != b {
		t.Error("expected deterministic hash")
	}
}

func TestHash_DifferentInputs(t *testing.T) {
	a := Hash("hello", SHA256)
	b := Hash("world", SHA256)
	if a == b {
		t.Error("expected different hashes")
	}
}

func TestHMAC(t *testing.T) {
	result := HMAC("message", "secret", SHA256)
	if result == "" {
		t.Error("expected non-empty HMAC")
	}
}

func TestHMAC_Verify(t *testing.T) {
	sig := HMAC("message", "secret", SHA256)
	if !Verify("message", "secret", sig, SHA256) {
		t.Error("expected valid HMAC")
	}
	if Verify("message", "wrong", sig, SHA256) {
		t.Error("expected invalid HMAC with wrong key")
	}
	if Verify("tampered", "secret", sig, SHA256) {
		t.Error("expected invalid HMAC with wrong message")
	}
}

func TestConstantTimeEqual(t *testing.T) {
	if !ConstantTimeEqual("abc", "abc") {
		t.Error("expected equal")
	}
	if ConstantTimeEqual("abc", "def") {
		t.Error("expected not equal")
	}
}

func TestHashBytes(t *testing.T) {
	result := HashBytes([]byte("hello"), SHA256)
	expected := Hash("hello", SHA256)
	if result != expected {
		t.Error("expected same result")
	}
}

func TestHMACBytes(t *testing.T) {
	result := HMACBytes([]byte("msg"), []byte("key"), SHA256)
	expected := HMAC("msg", "key", SHA256)
	if result != expected {
		t.Error("expected same result")
	}
}
