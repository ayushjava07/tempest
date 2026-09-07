package mmapring

import (
	"sync"
)

// RingBuffer provides an in-memory sequential circular buffer of framed entries.
type RingBuffer struct {
	mu         sync.RWMutex
	capacity   int
	data       []byte
	writePos   int
	oldestSeq  uint64
	nextSeq    uint64
	totalBytes uint64
}

// New creates a ring buffer with the specified byte capacity.
func New(capacity int) *RingBuffer {
	if capacity <= HeaderSize*2 {
		capacity = 64 * 1024 // 64 KB default
	}
	return &RingBuffer{
		capacity:  capacity,
		data:      make([]byte, capacity),
		oldestSeq: 1,
		nextSeq:   1,
	}
}

// Capacity returns the total allocated byte capacity.
func (r *RingBuffer) Capacity() int {
	return r.capacity
}

// Append writes payload as a framed record into the circular buffer.
func (r *RingBuffer) Append(payload []byte) (uint64, error) {
	frameLen := HeaderSize + len(payload)
	if frameLen > r.capacity {
		return 0, ErrBufferFull
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	seq := r.nextSeq
	r.nextSeq++

	hdr := Header{
		Magic:    FrameMagic,
		Seq:      seq,
		Length:   uint32(len(payload)),
		Checksum: ComputeChecksum(payload),
		Flags:    0,
	}

	// Write header
	var hdrBuf [HeaderSize]byte
	hdr.Encode(hdrBuf[:])

	r.writeBytesLocked(hdrBuf[:])
	r.writeBytesLocked(payload)
	r.totalBytes += uint64(frameLen)

	return seq, nil
}

func (r *RingBuffer) writeBytesLocked(p []byte) {
	for len(p) > 0 {
		avail := r.capacity - r.writePos
		chunk := len(p)
		if chunk > avail {
			chunk = avail
		}

		copy(r.data[r.writePos:r.writePos+chunk], p[:chunk])
		r.writePos = (r.writePos + chunk) % r.capacity
		p = p[chunk:]
	}
}

// ReadBytes reads n bytes starting from circular position offset.
func (r *RingBuffer) ReadBytes(offset int, dst []byte) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	r.readBytesLocked(offset, dst)
}

func (r *RingBuffer) readBytesLocked(offset int, dst []byte) {
	pos := offset % r.capacity
	for len(dst) > 0 {
		avail := r.capacity - pos
		chunk := len(dst)
		if chunk > avail {
			chunk = avail
		}
		copy(dst[:chunk], r.data[pos:pos+chunk])
		pos = (pos + chunk) % r.capacity
		dst = dst[chunk:]
	}
}

// ReadFrameAt parses and validates a frame at offset with CRC32 integrity check.
func (r *RingBuffer) ReadFrameAt(offset int) (Frame, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var hdrBuf [HeaderSize]byte
	r.readBytesLocked(offset, hdrBuf[:])

	hdr, err := DecodeHeader(hdrBuf[:])
	if err != nil {
		return Frame{}, 0, err
	}

	payload := make([]byte, hdr.Length)
	r.readBytesLocked(offset+HeaderSize, payload)

	if ComputeChecksum(payload) != hdr.Checksum {
		return Frame{}, 0, ErrChecksumFailed
	}

	nextOffset := (offset + HeaderSize + int(hdr.Length)) % r.capacity
	return Frame{
		Header:  hdr,
		Payload: payload,
	}, nextOffset, nil
}

// Snapshot extracts all currently decodable frames in sequence.
func (r *RingBuffer) Snapshot() []Frame {
	cursor := r.NewCursor(0)
	var frames []Frame

	for {
		f, ok, err := cursor.Next()
		if err != nil || !ok {
			break
		}
		frames = append(frames, f)
	}

	return frames
}

// Stats returns internal operational metrics.
func (r *RingBuffer) Stats() (capacity int, writePos int, nextSeq uint64, totalBytes uint64) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.capacity, r.writePos, r.nextSeq, r.totalBytes
}
