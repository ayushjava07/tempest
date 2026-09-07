package crypto

import (
	"testing"
)

func TestAESEncryptDecrypt(t *testing.T) {
	key, _ := GenerateAESKey()
	plaintext := []byte("hello world")
	ciphertext, err := AESEncrypt(key, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := AESDecrypt(key, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(decrypted) != string(plaintext) {
		t.Error("decryption failed")
	}
}

func TestAESDecryptWrongKey(t *testing.T) {
	key1, _ := GenerateAESKey()
	key2, _ := GenerateAESKey()
	ciphertext, _ := AESEncrypt(key1, []byte("test"))
	_, err := AESDecrypt(key2, ciphertext)
	if err == nil {
		t.Error("expected error with wrong key")
	}
}

func TestRSAEncryptDecrypt(t *testing.T) {
	priv, _ := GenerateRSAKeyPair(2048)
	plaintext := []byte("secret message")
	ciphertext, err := RSAEncrypt(&priv.PublicKey, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := RSADecrypt(priv, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(decrypted) != string(plaintext) {
		t.Error("decryption failed")
	}
}

func TestKeyEncoding(t *testing.T) {
	priv, _ := GenerateRSAKeyPair(2048)
	pemPriv := EncodePrivateKey(priv)
	decoded, err := DecodePrivateKey(pemPriv)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.N.Cmp(priv.N) != 0 {
		t.Error("private key mismatch")
	}
	pemPub := EncodePublicKey(&priv.PublicKey)
	decodedPub, err := DecodePublicKey(pemPub)
	if err != nil {
		t.Fatal(err)
	}
	if decodedPub.N.Cmp(priv.N) != 0 {
		t.Error("public key mismatch")
	}
}

func TestHash(t *testing.T) {
	h := Hash([]byte("test"))
	if len(h) != 64 {
		t.Errorf("expected 64 chars, got %d", len(h))
	}
	if h != Hash([]byte("test")) {
		t.Error("hash not deterministic")
	}
}