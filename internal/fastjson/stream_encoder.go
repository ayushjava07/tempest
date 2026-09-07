package fastjson

import (
	"strconv"
	"time"
)

// StreamEncoder writes JSON tokens directly into an underlying Buffer with zero heap allocations.
type StreamEncoder struct {
	buf       *Buffer
	needComma []bool // comma tracking stack for nested containers
}

// NewStreamEncoder creates a stream encoder targeting buf.
func NewStreamEncoder(buf *Buffer) *StreamEncoder {
	return &StreamEncoder{
		buf:       buf,
		needComma: make([]bool, 0, 8),
	}
}

// Reset re-initializes the encoder on a buffer.
func (e *StreamEncoder) Reset(buf *Buffer) {
	e.buf = buf
	e.needComma = e.needComma[:0]
}

func (e *StreamEncoder) writeSeparator() {
	if len(e.needComma) > 0 && e.needComma[len(e.needComma)-1] {
		e.buf.AppendByte(',')
	}
}

// BeginObject writes '{' and pushes a new object scope.
func (e *StreamEncoder) BeginObject() {
	e.writeSeparator()
	e.buf.AppendByte('{')
	e.needComma = append(e.needComma, false)
}

// EndObject writes '}' and pops object scope.
func (e *StreamEncoder) EndObject() {
	if len(e.needComma) > 0 {
		e.needComma = e.needComma[:len(e.needComma)-1]
	}
	e.buf.AppendByte('}')
	if len(e.needComma) > 0 {
		e.needComma[len(e.needComma)-1] = true
	}
}

// BeginArray writes '[' and pushes an array scope.
func (e *StreamEncoder) BeginArray() {
	e.writeSeparator()
	e.buf.AppendByte('[')
	e.needComma = append(e.needComma, false)
}

// EndArray writes ']' and pops array scope.
func (e *StreamEncoder) EndArray() {
	if len(e.needComma) > 0 {
		e.needComma = e.needComma[:len(e.needComma)-1]
	}
	e.buf.AppendByte(']')
	if len(e.needComma) > 0 {
		e.needComma[len(e.needComma)-1] = true
	}
}

// Key writes a property key with quotes and colon: "key":
func (e *StreamEncoder) Key(key string) {
	e.writeSeparator()
	e.buf.AppendByte('"')
	EscapeString(e.buf, key)
	e.buf.AppendString(`":`)
	if len(e.needComma) > 0 {
		e.needComma[len(e.needComma)-1] = false
	}
}

// WriteString writes an escaped string enclosed in quotes.
func (e *StreamEncoder) WriteString(s string) {
	e.writeSeparator()
	e.buf.AppendByte('"')
	EscapeString(e.buf, s)
	e.buf.AppendByte('"')
	if len(e.needComma) > 0 {
		e.needComma[len(e.needComma)-1] = true
	}
}

// WriteInt64 writes an integer.
func (e *StreamEncoder) WriteInt64(v int64) {
	e.writeSeparator()
	e.buf.buf = strconv.AppendInt(e.buf.buf, v, 10)
	if len(e.needComma) > 0 {
		e.needComma[len(e.needComma)-1] = true
	}
}

// WriteUint64 writes an unsigned integer.
func (e *StreamEncoder) WriteUint64(v uint64) {
	e.writeSeparator()
	e.buf.buf = strconv.AppendUint(e.buf.buf, v, 10)
	if len(e.needComma) > 0 {
		e.needComma[len(e.needComma)-1] = true
	}
}

// WriteFloat64 writes a float.
func (e *StreamEncoder) WriteFloat64(v float64) {
	e.writeSeparator()
	e.buf.buf = strconv.AppendFloat(e.buf.buf, v, 'f', -1, 64)
	if len(e.needComma) > 0 {
		e.needComma[len(e.needComma)-1] = true
	}
}

// WriteBool writes true or false.
func (e *StreamEncoder) WriteBool(v bool) {
	e.writeSeparator()
	if v {
		e.buf.AppendString("true")
	} else {
		e.buf.AppendString("false")
	}
	if len(e.needComma) > 0 {
		e.needComma[len(e.needComma)-1] = true
	}
}

// WriteNull writes null.
func (e *StreamEncoder) WriteNull() {
	e.writeSeparator()
	e.buf.AppendString("null")
	if len(e.needComma) > 0 {
		e.needComma[len(e.needComma)-1] = true
	}
}

// WriteTimeRFC3339 writes a formatted timestamp string.
func (e *StreamEncoder) WriteTimeRFC3339(t time.Time) {
	e.writeSeparator()
	e.buf.AppendByte('"')
	e.buf.buf = t.AppendFormat(e.buf.buf, time.RFC3339Nano)
	e.buf.AppendByte('"')
	if len(e.needComma) > 0 {
		e.needComma[len(e.needComma)-1] = true
	}
}

// WriteStringMap writes a map[string]string as a JSON object.
func (e *StreamEncoder) WriteStringMap(m map[string]string) {
	e.BeginObject()
	for k, v := range m {
		e.Key(k)
		e.WriteString(v)
	}
	e.EndObject()
}
