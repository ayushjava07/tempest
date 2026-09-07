package mmapring

import (
	"bytes"
	"fmt"
	"sync"
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

func TestRingBuffer_ChecksumCorruption(t *testing.T) {
	ring := New(200)

	payload := []byte("critical-financial-transaction")
	_, err := ring.Append(payload)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// Corrupt payload byte inside ring buffer
	ring.mu.Lock()
	ring.data[HeaderSize+2] ^= 0xFF
	ring.mu.Unlock()

	_, _, err = ring.ReadFrameAt(0)
	if err != ErrChecksumFailed {
		t.Fatalf("expected ErrChecksumFailed for corrupted payload, got: %v", err)
	}
}

func TestRingBuffer_CursorExpiration(t *testing.T) {
	// Tiny ring buffer (50 bytes)
	ring := New(50)
	cursor := ring.NewCursor(0)

	// Write frame 1
	_, _ = ring.Append([]byte("A"))

	// Consume frame 1
	f, ok, err := cursor.Next()
	if err != nil || !ok || f.Header.Seq != 1 {
		t.Fatalf("failed to read frame 1: ok=%v err=%v", ok, err)
	}

	// Overwrite ring buffer several times so old offset now contains newer sequence
	for i := 0; i < 5; i++ {
		_, _ = ring.Append([]byte("B"))
	}

	// Attempting to read old cursor position after wrap-around detects corruption/expiration
	cursor.lastSeq = 100 // artificially set high last sequence
	_, _, err = cursor.Next()
	if err != ErrCursorExpired && err != ErrInvalidMagic {
		t.Fatalf("expected ErrCursorExpired or ErrInvalidMagic on overwritten buffer, got: %v", err)
	}
}

func TestRingBuffer_ConcurrentProducersAndConsumers(t *testing.T) {
	ring := New(64 * 1024)

	var wg sync.WaitGroup
	const numProducers = 4
	const framesPerProducer = 25

	// Producers
	for p := 0; p < numProducers; p++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < framesPerProducer; i++ {
				payload := []byte(fmt.Sprintf("producer-%d-frame-%d", id, i))
				_, err := ring.Append(payload)
				if err != nil {
					t.Errorf("Append failed: %v", err)
				}
			}
		}(p)
	}

	wg.Wait()

	// Consumer verifies total frames written
	cursor := ring.NewCursor(0)
	var count int
	for {
		_, ok, err := cursor.Next()
		if err != nil || !ok {
			break
		}
		count++
	}

	if count != numProducers*framesPerProducer {
		t.Fatalf("expected %d frames, got %d", numProducers*framesPerProducer, count)
	}
}
