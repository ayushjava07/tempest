package mmapring

import (
	"bytes"
	"fmt"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestHeader_EncodeDecode(t *testing.T) {
	hdr := Header{
		Magic:    FrameMagic,
		Seq:      1001,
		Length:   256,
		Checksum: 0xAABBCCDD,
		Flags:    4,
	}

	var buf [HeaderSize]byte
	hdr.Encode(buf[:])

	decoded, err := DecodeHeader(buf[:])
	if err != nil {
		t.Fatalf("DecodeHeader failed: %v", err)
	}

	if decoded != hdr {
		t.Fatalf("header mismatch: %+v vs %+v", decoded, hdr)
	}

	// Test invalid magic
	buf[0] = 0x00
	_, err = DecodeHeader(buf[:])
	if err != ErrInvalidMagic {
		t.Fatalf("expected ErrInvalidMagic, got %v", err)
	}
}

func TestRingBuffer_AppendAndWrapAround(t *testing.T) {
	// Small ring buffer (100 bytes) to force rapid wrap-around
	ring := New(100)
	cursor := ring.NewCursor(0)

	numFrames := 10
	for i := 0; i < numFrames; i++ {
		payload := []byte(fmt.Sprintf("msg-%02d", i))
		seq, err := ring.Append(payload)
		if err != nil {
			t.Fatalf("Append frame %d failed: %v", i, err)
		}
		if seq != uint64(i+1) {
			t.Fatalf("expected sequence %d, got %d", i+1, seq)
		}

		frame, ok, err := cursor.Next()
		if err != nil || !ok {
			t.Fatalf("Cursor.Next frame %d failed: ok=%v err=%v", i, ok, err)
		}
		if !bytes.Equal(frame.Payload, payload) {
			t.Fatalf("payload mismatch: expected %s, got %s", string(payload), string(frame.Payload))
		}
	}

	// No more frames available
	_, ok, err := cursor.Next()
	if err != nil || ok {
		t.Fatalf("expected no more frames, ok=%v err=%v", ok, err)
	}
}
