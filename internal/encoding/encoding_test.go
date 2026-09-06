package encoding

import (
	"testing"
)

func TestBase64EncodeDecode(t *testing.T) {
	data := []byte("hello world")
	encoded := Base64Encode(data)
	decoded, err := Base64Decode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != "hello world" {
		t.Errorf("expected hello world, got %s", decoded)
	}
}

func TestBase64URL(t *testing.T) {
	data := []byte("test data with special chars?=&")
	encoded := Base64URLEncode(data)
	decoded, err := Base64URLDecode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != string(data) {
		t.Error("roundtrip failed")
	}
}

func TestHexEncodeDecode(t *testing.T) {
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	encoded := HexEncode(data)
	if encoded != "deadbeef" {
		t.Errorf("expected deadbeef, got %s", encoded)
	}
	decoded, err := HexDecode(encoded)
	if err != nil {
		t.Fatal(err)
	}
	for i, b := range decoded {
		if b != data[i] {
			t.Error("roundtrip failed")
		}
	}
}

func TestHexDecode_Invalid(t *testing.T) {
	_, err := HexDecode("xyz")
	if err == nil {
		t.Error("expected error")
	}
}

func TestHexDump(t *testing.T) {
	data := []byte("Hello, World!")
	dump := HexDump(data, 16)
	if dump == "" {
		t.Error("expected non-empty dump")
	}
}

func TestXOR(t *testing.T) {
	data := []byte("hello")
	key := []byte("key")
	encoded := XOREncode(data, key)
	decoded := XORDecode(encoded, key)
	if string(decoded) != "hello" {
		t.Error("XOR roundtrip failed")
	}
}

func TestBase64Decode_Invalid(t *testing.T) {
	_, err := Base64Decode("!!!invalid!!!")
	if err == nil {
		t.Error("expected error")
	}
}
