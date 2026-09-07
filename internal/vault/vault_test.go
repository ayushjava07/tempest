package vault

import (
	"context"
	"fmt"
	"testing"
)

func TestVault_StoreRetrieve(t *testing.T) {
	key, _ := GenerateKey()
	v := New(key)
	if err := v.Store(context.Background(), "secret1", []byte("hello")); err != nil {
		t.Fatal(err)
	}
	val, err := v.Retrieve(context.Background(), "secret1")
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "hello" {
		t.Errorf("expected hello, got %s", val)
	}
}

func TestVault_Delete(t *testing.T) {
	key, _ := GenerateKey()
	v := New(key)
	v.Store(context.Background(), "s1", []byte("x"))
	if err := v.Delete(context.Background(), "s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := v.Retrieve(context.Background(), "s1"); err == nil {
		t.Error("expected not found")
	}
}

func TestVault_List(t *testing.T) {
	key, _ := GenerateKey()
	v := New(key)
	v.Store(context.Background(), "a", []byte("1"))
	v.Store(context.Background(), "b", []byte("2"))
	list, err := v.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Errorf("expected 2, got %d", len(list))
	}
}

func TestVault_NotFound(t *testing.T) {
	key, _ := GenerateKey()
	v := New(key)
	_, err := v.Retrieve(context.Background(), "missing")
	if err == nil {
		t.Error("expected error")
	}
}

func TestVault_KeyGeneration(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(key) != 32 {
		t.Errorf("expected 32 bytes, got %d", len(key))
	}
	key2, _ := GenerateKey()
	if string(key) == string(key2) {
		t.Error("expected different keys")
	}
}

func TestVault_EncodeDecode(t *testing.T) {
	key, _ := GenerateKey()
	encoded := EncodeKey(key)
	decoded, err := DecodeKey(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(key) != string(decoded) {
		t.Error("key mismatch")
	}
}

func TestVault_Concurrent(t *testing.T) {
	key, _ := GenerateKey()
	v := New(key)
	done := make(chan bool, 100)
	for i := 0; i < 100; i++ {
		go func(n int) {
			v.Store(context.Background(), fmt.Sprintf("s%d", n), []byte("val"))
			v.Retrieve(context.Background(), fmt.Sprintf("s%d", n))
			done <- true
		}(i)
	}
	for i := 0; i < 100; i++ {
		<-done
	}
}