package fastjson

import (
	"encoding/json"
	"testing"
)

func TestEscapeStringControlsAndUnicode(t *testing.T) {
	testCases := []string{
		"Simple plain text",
		`String with "quotes" and \backslashes\`,
		"Line1\nLine2\r\nLine3\tTabbed\b\f",
		"Control chars: \x00\x01\x02\x1f",
		"Unicode text: こんにちは, 世界! 🚀 Workflow ⚡",
		`{"nested":"json_string"}`,
		"",
	}

	for i, tc := range testCases {
		buf := Acquire()

		buf.AppendByte('"')
		EscapeString(buf, tc)
		buf.AppendByte('"')

		var decoded string
		if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
			t.Fatalf("case %d (%q): json.Unmarshal failed on escaped output %s: %v",
				i, tc, string(buf.Bytes()), err)
		}

		if decoded != tc {
			t.Fatalf("case %d: decoded string mismatch\ngot:  %q\nwant: %q", i, decoded, tc)
		}

		Release(buf)
	}
}
