package providers

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEnvProviderLookup(t *testing.T) {
	ctx := context.Background()
	p := NewEnvProvider("TEMPEST_")

	// Pre-populate an override
	err := p.PutSecret(ctx, "db.password", []byte("s3cretP@ss"))
	if err != nil {
		t.Fatalf("put failed: %v", err)
	}

	sec, err := p.GetSecret(ctx, "db.password")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if string(sec.Value) != "s3cretP@ss" {
		t.Fatalf("expected s3cretP@ss, got %s", string(sec.Value))
	}

	// Test list
	keys, err := p.ListSecrets(ctx)
	if err != nil || len(keys) != 1 || keys[0] != "db.password" {
		t.Fatalf("unexpected list: %v", keys)
	}

	// Test delete
	err = p.DeleteSecret(ctx, "db.password")
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	_, err = p.GetSecret(ctx, "db.password")
	if err == nil {
		t.Fatalf("expected not found error after delete")
	}
}

func TestFileProviderLookupAndPathTraversal(t *testing.T) {
	ctx := context.Background()
	tmpDir, err := os.MkdirTemp("", "vault-file-test-*")
	if err != nil {
		t.Fatalf("temp dir failed: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	fp, err := NewFileProvider(tmpDir)
	if err != nil {
		t.Fatalf("new file provider failed: %v", err)
	}

	// Store secret
	err = fp.PutSecret(ctx, "api-key.txt", []byte("api-token-999"))
	if err != nil {
		t.Fatalf("put secret failed: %v", err)
	}

	sec, err := fp.GetSecret(ctx, "api-key.txt")
	if err != nil || string(sec.Value) != "api-token-999" {
		t.Fatalf("get secret failed: %v, val: %s", err, string(sec.Value))
	}

	// Path traversal attempt should be rejected
	_, err = fp.GetSecret(ctx, "../../../etc/passwd")
	if err == nil {
		t.Fatalf("expected path traversal to be rejected")
	}

	// Subdir path should work safely
	err = fp.PutSecret(ctx, filepath.Join("nested", "token.json"), []byte(`{"tok":"123"}`))
	if err != nil {
		t.Fatalf("nested put failed: %v", err)
	}
	sec, err = fp.GetSecret(ctx, filepath.Join("nested", "token.json"))
	if err != nil || !bytes.Equal(sec.Value, []byte(`{"tok":"123"}`)) {
		t.Fatalf("nested get failed: %v", err)
	}
}

func TestHashiCorpVaultKVv2AndTransit(t *testing.T) {
	ctx := context.Background()
	hv := NewHashiCorpVaultProvider("http://vault.internal:8200", "root-token-xyz", "secret/data")

	// Store version 1
	err := hv.PutSecret(ctx, "stripe/webhook_key", []byte("whsec_111"))
	if err != nil {
		t.Fatalf("put v1 failed: %v", err)
	}

	// Store version 2
	err = hv.PutSecret(ctx, "stripe/webhook_key", []byte("whsec_222"))
	if err != nil {
		t.Fatalf("put v2 failed: %v", err)
	}

	sec, err := hv.GetSecret(ctx, "stripe/webhook_key")
	if err != nil {
		t.Fatalf("get secret failed: %v", err)
	}
	if string(sec.Value) != "whsec_222" || sec.Metadata.Version != 2 {
		t.Fatalf("expected version 2 with whsec_222, got ver=%d val=%s", sec.Metadata.Version, string(sec.Value))
	}

	// Test Transit encryption
	plaintext := []byte("confidential credit card data")
	cipherText, err := hv.TransitEncrypt("orders-key", plaintext)
	if err != nil {
		t.Fatalf("transit encrypt failed: %v", err)
	}
	if !bytes.Contains([]byte(cipherText), []byte("vault:v1:")) {
		t.Fatalf("expected vault:v1: prefix in ciphertext")
	}

	decrypted, err := hv.TransitDecrypt("orders-key", cipherText)
	if err != nil {
		t.Fatalf("transit decrypt failed: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted mismatch: got %s", string(decrypted))
	}
}

func TestProviderRegistryFallback(t *testing.T) {
	ctx := context.Background()
	reg := NewRegistry()

	envP := NewEnvProvider("TEST_")
	vaultP := NewHashiCorpVaultProvider("http://vault", "tok", "secret")

	reg.Register(envP)
	reg.Register(vaultP)

	// Secret only exists in vaultP
	err := vaultP.PutSecret(ctx, "backend-service-token", []byte("service-jwt-abc"))
	if err != nil {
		t.Fatalf("put failed: %v", err)
	}

	sec, providerName, err := reg.ResolveSecret(ctx, "backend-service-token")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if providerName != "hashicorp-vault" {
		t.Fatalf("expected resolved provider hashicorp-vault, got %s", providerName)
	}
	if string(sec.Value) != "service-jwt-abc" {
		t.Fatalf("unexpected secret value: %s", string(sec.Value))
	}
}

func TestCachedProviderHitsAndMisses(t *testing.T) {
	ctx := context.Background()
	envP := NewEnvProvider("TEST_")
	_ = envP.PutSecret(ctx, "frequent-secret", []byte("fast-data"))

	cached, err := NewCachedProvider(envP, 500*time.Millisecond, 0.05, nil)
	if err != nil {
		t.Fatalf("cached provider init failed: %v", err)
	}

	// First call -> cache miss
	sec1, err := cached.GetSecret(ctx, "frequent-secret")
	if err != nil || string(sec1.Value) != "fast-data" {
		t.Fatalf("get secret failed: %v", err)
	}

	// Second and third calls -> cache hits
	for i := 0; i < 5; i++ {
		sec, err := cached.GetSecret(ctx, "frequent-secret")
		if err != nil || string(sec.Value) != "fast-data" {
			t.Fatalf("cached get failed: %v", err)
		}
	}

	hits, misses, _ := cached.Stats()
	if misses != 1 {
		t.Fatalf("expected 1 miss, got %d", misses)
	}
	if hits != 5 {
		t.Fatalf("expected 5 hits, got %d", hits)
	}
}
