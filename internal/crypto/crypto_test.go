package crypto

import (
	"bytes"
	"crypto/rand"
	"testing"
)

func TestCrypto_EncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)

	plaintext := []byte("confidential-workflow-token-12345")

	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	if bytes.Equal(plaintext, ciphertext) {
		t.Errorf("ciphertext should not match plaintext")
	}

	decrypted, err := Decrypt(key, ciphertext)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("expected %s, got %s", plaintext, decrypted)
	}
}

func TestCrypto_TamperedCiphertext(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)

	plaintext := []byte("tamper-test")
	ciphertext, err := Encrypt(key, plaintext)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Tamper with ciphertext
	ciphertext[len(ciphertext)-1] ^= 0xFF

	_, err = Decrypt(key, ciphertext)
	if err != ErrDecryptionFailed {
		t.Errorf("expected ErrDecryptionFailed on tampered ciphertext, got %v", err)
	}
}

func TestCrypto_InvalidKeyLength(t *testing.T) {
	shortKey := []byte("short-key")
	_, err := Encrypt(shortKey, []byte("data"))
	if err != ErrInvalidKeyLength {
		t.Errorf("expected ErrInvalidKeyLength, got %v", err)
	}

	_, err = Decrypt(shortKey, []byte("data"))
	if err != ErrInvalidKeyLength {
		t.Errorf("expected ErrInvalidKeyLength, got %v", err)
	}
}

func TestCrypto_ConstantTimeCompare(t *testing.T) {
	a := []byte("secure-token-1")
	b := []byte("secure-token-1")
	c := []byte("secure-token-2")

	if !ConstantTimeCompare(a, b) {
		t.Errorf("expected identical slices to match")
	}
	if ConstantTimeCompare(a, c) {
		t.Errorf("expected different slices not to match")
	}
}
