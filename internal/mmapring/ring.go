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
