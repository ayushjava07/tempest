package mmapring

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
)

var (
	ErrInvalidMagic   = errors.New("mmapring: invalid frame magic bytes")
	ErrCorruptHeader  = errors.New("mmapring: frame header corrupted or too short")
	ErrChecksumFailed = errors.New("mmapring: frame payload checksum verification failed")
	ErrBufferFull     = errors.New("mmapring: ring buffer capacity exceeded")
	ErrBufferEmpty    = errors.New("mmapring: no entries available")
	ErrCursorExpired  = errors.New("mmapring: consumer cursor has been overwritten by ring buffer")
)

const (
	FrameMagic uint32 = 0x544D5052 // "TMPR"
	HeaderSize int    = 22         // 4 + 8 + 4 + 4 + 2
)

// Header defines the binary frame envelope metadata.
type Header struct {
	Magic    uint32
	Seq      uint64
	Length   uint32
	Checksum uint32
	Flags    uint16
}

// EncodeHeader serializes the header into a 22-byte slice.
func (h Header) Encode(dst []byte) {
	binary.BigEndian.PutUint32(dst[0:4], h.Magic)
	binary.BigEndian.PutUint64(dst[4:12], h.Seq)
	binary.BigEndian.PutUint32(dst[12:16], h.Length)
	binary.BigEndian.PutUint32(dst[16:20], h.Checksum)
	binary.BigEndian.PutUint16(dst[20:22], h.Flags)
}

// DecodeHeader deserializes header from a byte slice.
func DecodeHeader(src []byte) (Header, error) {
	if len(src) < HeaderSize {
		return Header{}, ErrCorruptHeader
	}

	magic := binary.BigEndian.Uint32(src[0:4])
	if magic != FrameMagic {
		return Header{}, ErrInvalidMagic
	}

	return Header{
		Magic:    magic,
		Seq:      binary.BigEndian.Uint64(src[4:12]),
		Length:   binary.BigEndian.Uint32(src[12:16]),
		Checksum: binary.BigEndian.Uint32(src[16:20]),
		Flags:    binary.BigEndian.Uint16(src[20:22]),
	}, nil
}

// Frame represents a self-contained log entry.
type Frame struct {
	Header  Header
	Payload []byte
}

// ComputeChecksum calculates CRC32 IEEE checksum for data.
func ComputeChecksum(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}
