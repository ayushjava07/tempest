package wal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"sync"
)

var (
	ErrChecksumMismatch = errors.New("wal frame checksum mismatch")
	ErrCorruptHeader    = errors.New("wal frame header corrupt")
)

const magicHeader uint32 = 0x544D5057 // 'TMPW' (Tempest WAL)

// Record represents a single state mutation record in the log.
type Record struct {
	SeqID uint64
	Type  uint16
	Data  []byte
}

// Log represents an append-only write-ahead log.
type Log struct {
	mu     sync.Mutex
	file   *os.File
	nextSeq uint64
}

// Open opens or creates a WAL file at path.
func Open(path string) (*Log, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open wal: %w", err)
	}

	l := &Log{
		file: f,
	}
	return l, nil
}

// Close closes the WAL file cleanly.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}

// Append writes a new record with a frame header and crc32 checksum.
// Frame format:
// [Magic (4B)][SeqID (8B)][Type (2B)][Len (4B)][Payload (NB)][CRC32 (4B)]
func (l *Log) Append(recType uint16, data []byte) (uint64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.nextSeq++
	seq := l.nextSeq

	buf := new(bytes.Buffer)
	_ = binary.Write(buf, binary.BigEndian, magicHeader)
	_ = binary.Write(buf, binary.BigEndian, seq)
	_ = binary.Write(buf, binary.BigEndian, recType)
	_ = binary.Write(buf, binary.BigEndian, uint32(len(data)))
	buf.Write(data)

	checksum := crc32.ChecksumIEEE(buf.Bytes())
	_ = binary.Write(buf, binary.BigEndian, checksum)

	if _, err := l.file.Write(buf.Bytes()); err != nil {
		return 0, fmt.Errorf("write wal record: %w", err)
	}

	return seq, nil
}

// Replay reads all valid records from the start of the log file.
func Replay(path string) ([]Record, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var records []Record
	headerBuf := make([]byte, 18) // Magic(4) + Seq(8) + Type(2) + Len(4)

	for {
		_, err := io.ReadFull(f, headerBuf)
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read header: %w", err)
		}

		magic := binary.BigEndian.Uint32(headerBuf[0:4])
		if magic != magicHeader {
			return nil, ErrCorruptHeader
		}

		seq := binary.BigEndian.Uint64(headerBuf[4:12])
		recType := binary.BigEndian.Uint16(headerBuf[12:14])
		dataLen := binary.BigEndian.Uint32(headerBuf[14:18])

		payload := make([]byte, dataLen)
		if _, err := io.ReadFull(f, payload); err != nil {
			return nil, fmt.Errorf("read payload: %w", err)
		}

		var expectedCRC uint32
		if err := binary.Read(f, binary.BigEndian, &expectedCRC); err != nil {
			return nil, fmt.Errorf("read checksum: %w", err)
		}

		// Verify checksum over header + payload
		checkBuf := append(headerBuf, payload...)
		actualCRC := crc32.ChecksumIEEE(checkBuf)
		if actualCRC != expectedCRC {
			return nil, ErrChecksumMismatch
		}

		records = append(records, Record{
			SeqID: seq,
			Type:  recType,
			Data:  payload,
		})
	}

	return records, nil
}
