package hasher

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"hash"
)

type Algorithm int

const (
	SHA256 Algorithm = iota
	SHA512
)

func Hash(data string, algo Algorithm) string {
	return HashBytes([]byte(data), algo)
}

func HashBytes(data []byte, algo Algorithm) string {
	var h hash.Hash
	switch algo {
	case SHA512:
		h = sha512.New()
	default:
		h = sha256.New()
	}
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func HMAC(data, key string, algo Algorithm) string {
	return HMACBytes([]byte(data), []byte(key), algo)
}

func HMACBytes(data, key []byte, algo Algorithm) string {
	var h func() hash.Hash
	switch algo {
	case SHA512:
		h = sha512.New
	default:
		h = sha256.New
	}
	mac := hmac.New(h, key)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

func Verify(data, key, expected string, algo Algorithm) bool {
	return HMAC(data, key, algo) == expected
}

func VerifyBytes(data, key, expected []byte, algo Algorithm) bool {
	return hmac.Equal([]byte(HMACBytes(data, key, algo)), expected)
}

func ConstantTimeEqual(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}
