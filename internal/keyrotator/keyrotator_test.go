package keyrotator

import (
	"bytes"
	"errors"
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestEnvelope_BinarySerialization(t *testing.T) {
	env := &Envelope{
		Version:    42,
		KeyID:      "key-v42-abcd1234efgh",
		Nonce:      []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		Ciphertext: []byte("encrypted-payload-bytes"),
	}

	raw := env.MarshalBinary()
	parsed, err := UnmarshalBinary(raw)
	if err != nil {
		t.Fatalf("UnmarshalBinary failed: %v", err)
	}

	if parsed.Version != env.Version {
		t.Fatalf("version mismatch: %d vs %d", parsed.Version, env.Version)
	}
	if parsed.KeyID != env.KeyID {
		t.Fatalf("keyID mismatch: %s vs %s", parsed.KeyID, env.KeyID)
	}
	if !bytes.Equal(parsed.Nonce, env.Nonce) {
		t.Fatalf("nonce mismatch")
	}
	if !bytes.Equal(parsed.Ciphertext, env.Ciphertext) {
		t.Fatalf("ciphertext mismatch")
	}
}

func TestRotator_EncryptDecryptAndRotate(t *testing.T) {
	kek := []byte("master-secret-key-32-bytes-long!")
	rotator, err := New(kek, RotationPolicy{})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	plaintext := []byte("top-secret-workflow-payload")
	aad := []byte("workflow-id-12345")

	// Encrypt under Key V1
	env1, err := rotator.Encrypt(plaintext, aad)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}
	v1KeyID := rotator.ActiveKeyID()

	// Decrypt under V1
	dec1, err := rotator.Decrypt(env1, aad)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}
	if !bytes.Equal(dec1, plaintext) {
		t.Fatalf("plaintext mismatch: expected %s, got %s", string(plaintext), string(dec1))
	}

	// AAD mismatch must fail authentication
	_, err = rotator.Decrypt(env1, []byte("wrong-aad"))
	if err == nil || !errors.Is(err, ErrDecryptionFail) {
		t.Fatalf("expected ErrDecryptionFail for wrong AAD, got: %v", err)
	}

	// Rotate to Key V2
	_, err = rotator.Rotate()
	if err != nil {
		t.Fatalf("Rotate failed: %v", err)
	}
	v2KeyID := rotator.ActiveKeyID()
	if v1KeyID == v2KeyID {
		t.Fatalf("expected active key to change after rotation")
	}

	// Old ciphertext (V1) must still be decryptable
	decOld, err := rotator.Decrypt(env1, aad)
	if err != nil {
		t.Fatalf("Decrypting old ciphertext failed: %v", err)
	}
	if !bytes.Equal(decOld, plaintext) {
		t.Fatalf("old plaintext mismatch")
	}

	// ReEncrypt old ciphertext to V2
	env2, migrated, err := rotator.ReEncrypt(env1, aad)
	if err != nil || !migrated {
		t.Fatalf("ReEncrypt failed: migrated=%v err=%v", migrated, err)
	}
	if env2.KeyID != v2KeyID {
		t.Fatalf("expected new envelope to have active key %s, got %s", v2KeyID, env2.KeyID)
	}

	// Re-encrypting an already-latest envelope returns migrated=false
	_, migrated2, err := rotator.ReEncrypt(env2, aad)
	if err != nil || migrated2 {
		t.Fatalf("re-encrypt of latest envelope should not migrate")
	}
}

func TestRotator_KeyRevocation(t *testing.T) {
	kek := []byte("master-secret-key-32-bytes-long!")
	rotator, _ := New(kek, RotationPolicy{})

	plaintext := []byte("confidential-pii-record")
	env, _ := rotator.Encrypt(plaintext, nil)
	keyToRevoke := env.KeyID

	// Rotate so active key is not the revoked key
	_, _ = rotator.Rotate()

	// Revoke old key (cryptographic shredding)
	err := rotator.RevokeKey(keyToRevoke)
	if err != nil {
		t.Fatalf("RevokeKey failed: %v", err)
	}

	// Decrypting data encrypted with revoked key must fail permanently
	_, err = rotator.Decrypt(env, nil)
	if err != ErrKeyRevoked {
		t.Fatalf("expected ErrKeyRevoked, got: %v", err)
	}
}

func TestRotator_Concurrency(t *testing.T) {
	kek := []byte("master-secret-key-32-bytes-long!")
	rotator, _ := New(kek, RotationPolicy{})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			data := []byte("parallel-data")
			for j := 0; j < 25; j++ {
				env, err := rotator.Encrypt(data, nil)
				if err != nil {
					t.Errorf("concurrent encrypt failed: %v", err)
					return
				}
				dec, err := rotator.Decrypt(env, nil)
				if err != nil || !bytes.Equal(dec, data) {
					t.Errorf("concurrent decrypt failed: %v", err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
}
