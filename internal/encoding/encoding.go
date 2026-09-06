package encoding

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

func Base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func Base64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

func Base64URLEncode(data []byte) string {
	return base64.URLEncoding.EncodeToString(data)
}

func Base64URLDecode(s string) ([]byte, error) {
	return base64.URLEncoding.DecodeString(s)
}

func HexEncode(data []byte) string {
	return hex.EncodeToString(data)
}

func HexDecode(s string) ([]byte, error) {
	return hex.DecodeString(s)
}

func HexDump(data []byte, bytesPerLine int) string {
	if bytesPerLine <= 0 {
		bytesPerLine = 16
	}
	var result string
	for i := 0; i < len(data); i += bytesPerLine {
		end := i + bytesPerLine
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]
		hexPart := ""
		asciiPart := ""
		for j, b := range chunk {
			hexPart += fmt.Sprintf("%02x ", b)
			if b >= 32 && b <= 126 {
				asciiPart += string(b)
			} else {
				asciiPart += "."
			}
			if j == bytesPerLine/2-1 {
				hexPart += " "
			}
		}
		result += fmt.Sprintf("%04x: %-48s |%s|\n", i, hexPart, asciiPart)
	}
	return result
}

func XOREncode(data, key []byte) []byte {
	result := make([]byte, len(data))
	for i, b := range data {
		result[i] = b ^ key[i%len(key)]
	}
	return result
}

func XORDecode(data, key []byte) []byte {
	return XOREncode(data, key)
}
