package vault

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func generateKey() []byte {
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	return k
}

func TestVault_PutAndGet(t *testing.T) {
	k1 := generateKey()
	ring, err := NewKeyRing(k1)
	if err != nil {
		t.Fatalf("NewKeyRing failed: %v", err)
	}

	v := NewVault(ring)
	ctx := context.Background()

	path := "secrets/prod/stripe_key"
	plaintext := []byte("sk_live_51M0abcdef123456789")

	sec, err := v.Put(ctx, path, plaintext)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	if sec.KeyVersion != 1 {
		t.Errorf("expected key version 1, got %d", sec.KeyVersion)
	}

	got, err := v.Get(ctx, path)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("decrypted secret mismatch: %s != %s", string(got), string(plaintext))
	}

	// Unknown path
	_, err = v.Get(ctx, "secrets/nonexistent")
	if !errors.Is(err, ErrSecretNotFound) {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestVault_KeyRotationAndReencryption(t *testing.T) {
	k1 := generateKey()
	ring, _ := NewKeyRing(k1)
	v := NewVault(ring)
	ctx := context.Background()

	// Store secret with key version 1
	path := "secrets/database"
	plaintext := []byte("postgres://admin:secret@pg:5432/tempest")
	sec1, _ := v.Put(ctx, path, plaintext)
	if sec1.KeyVersion != 1 {
		t.Fatalf("expected version 1")
	}

	// Rotate key to version 2
	k2 := generateKey()
	count, err := v.RotateAndReencrypt(ctx, k2)
	if err != nil {
		t.Fatalf("RotateAndReencrypt failed: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 reencrypted secret, got %d", count)
	}

	// Secret should now be encrypted with version 2
	sec2 := v.secrets[path]
	if sec2.KeyVersion != 2 {
		t.Errorf("expected key version 2 after rotation, got %d", sec2.KeyVersion)
	}

	// Decryption should still yield original plaintext
	got, err := v.Get(ctx, path)
	if err != nil {
		t.Fatalf("Get after rotation failed: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("plaintext mismatch after rotation: %s", string(got))
	}
}

func TestVault_TamperedCiphertext(t *testing.T) {
	ring, _ := NewKeyRing(generateKey())
	v := NewVault(ring)
	ctx := context.Background()

	path := "secrets/test"
	_, _ = v.Put(ctx, path, []byte("sensitive-data"))

	// Tamper with ciphertext byte
	v.secrets[path].Ciphertext[0] ^= 0xFF

	_, err := v.Get(ctx, path)
	if !errors.Is(err, ErrCiphertextCorrupt) {
		t.Fatalf("expected ErrCiphertextCorrupt for tampered ciphertext, got %v", err)
	}
}

func TestVault_InvalidKey(t *testing.T) {
	_, err := NewKeyRing([]byte("short-key"))
	if !errors.Is(err, ErrInvalidKeyLength) {
		t.Errorf("expected ErrInvalidKeyLength, got %v", err)
	}

	ring, _ := NewKeyRing(generateKey())
	_, err = ring.Rotate([]byte("short-key-too"))
	if !errors.Is(err, ErrInvalidKeyLength) {
		t.Errorf("expected ErrInvalidKeyLength on rotate, got %v", err)
	}
}

func TestVault_Concurrency(t *testing.T) {
	ring, _ := NewKeyRing(generateKey())
	v := NewVault(ring)
	concurrency := 10

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			path := fmt.Sprintf("secrets/worker-%d", id)
			data := []byte(fmt.Sprintf("secret-val-%d", id))
			_, err := v.Put(context.Background(), path, data)
			if err != nil {
				t.Errorf("Put failed: %v", err)
				return
			}
			dec, err := v.Get(context.Background(), path)
			if err != nil || !bytes.Equal(dec, data) {
				t.Errorf("Get failed in worker %d", id)
			}
		}(i)
	}
	wg.Wait()
}
