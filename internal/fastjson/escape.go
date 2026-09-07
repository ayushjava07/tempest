package fastjson

import (
	"fmt"
)

var hexChars = "0123456789abcdef"

// EscapeString appends the escaped form of s into buf without enclosing quotes.
func EscapeString(buf *Buffer, s string) {
	start := 0
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b >= 0x20 && b != '\\' && b != '"' {
			continue
		}

		// Flush preceding unescaped chunk
		if i > start {
			buf.AppendString(s[start:i])
		}

		switch b {
		case '\\':
			buf.AppendString(`\\`)
		case '"':
			buf.AppendString(`\"`)
		case '\n':
			buf.AppendString(`\n`)
		case '\r':
			buf.AppendString(`\r`)
		case '\t':
			buf.AppendString(`\t`)
		case '\b':
			buf.AppendString(`\b`)
		case '\f':
			buf.AppendString(`\f`)
		default:
			// Control character < 0x20: \u00xx
			buf.AppendString(`\u00`)
			buf.AppendByte(hexChars[b>>4])
			buf.AppendByte(hexChars[b&0xF])
		}
		start = i + 1
	}

	if start < len(s) {
		buf.AppendString(s[start:])
	}
}

// FormatWorkflowEventJSON writes a standardized workflow event directly to JSON bytes.
func FormatWorkflowEventJSON(seq uint64, eventType, stepID string, payload map[string]string) []byte {
	buf := Acquire()
	enc := NewStreamEncoder(buf)

	enc.BeginObject()
	enc.Key("seq")
	enc.WriteUint64(seq)
	enc.Key("type")
	enc.WriteString(eventType)
	if stepID != "" {
		enc.Key("step_id")
		enc.WriteString(stepID)
	}
	if len(payload) > 0 {
		enc.Key("payload")
		enc.WriteStringMap(payload)
	}
	enc.EndObject()

	out := append([]byte(nil), buf.Bytes()...)
	Release(buf)
	return out
}

func init() {
	_ = fmt.Sprintf("")
}
