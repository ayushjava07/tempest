package mmapring

import (
	"sync"
)

// Cursor tracks an independent consumer's read position in the ring buffer.
type Cursor struct {
	ring    *RingBuffer
	offset  int
	lastSeq uint64
	mu      sync.Mutex
}

// NewCursor creates a cursor starting at specified byte offset.
func (r *RingBuffer) NewCursor(startOffset int) *Cursor {
	return &Cursor{
		ring:   r,
		offset: startOffset % r.capacity,
	}
}

// Next attempts to read the next complete frame. Returns (frame, ok, err).
func (c *Cursor) Next() (Frame, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ring.mu.RLock()
	currentWritePos := c.ring.writePos
	c.ring.mu.RUnlock()

	// If cursor is at the writer head, no new frames are ready
	if c.offset == currentWritePos {
		return Frame{}, false, nil
	}

	frame, nextOffset, err := c.ring.ReadFrameAt(c.offset)
	if err != nil {
		return Frame{}, false, err
	}

	// Sequence must be strictly increasing
	if c.lastSeq > 0 && frame.Header.Seq <= c.lastSeq {
		return Frame{}, false, ErrCursorExpired
	}

	c.offset = nextOffset
	c.lastSeq = frame.Header.Seq
	return frame, true, nil
}

// Offset returns the current circular byte offset.
func (c *Cursor) Offset() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.offset
}

// LastSeq returns the most recently consumed sequence number.
func (c *Cursor) LastSeq() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastSeq
}
