package archiver

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
)

// Unpacker decompresses and deserializes cold storage archives.
type Unpacker struct{}

// NewUnpacker creates an archive unpacker.
func NewUnpacker() *Unpacker {
	return &Unpacker{}
}

// Unpack verifies manifest SHA-256 and extracts all records from reader.
func (u *Unpacker) Unpack(r io.Reader, manifest *Manifest) ([]ArchiveRecord, error) {
	if manifest == nil {
		return nil, ErrInvalidArchive
	}

	// Read entire stream and verify SHA256
	rawCompressed, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	actualHash := sha256.Sum256(rawCompressed)
	actualDigest := hex.EncodeToString(actualHash[:])
	if manifest.SHA256 != "" && actualDigest != manifest.SHA256 {
		return nil, ErrChecksumMismatch
	}

	var decompReader io.Reader
	switch manifest.Codec {
	case CodecNone:
		decompReader = bytes.NewReader(rawCompressed)
	case CodecGzip:
		gzReader, err := gzip.NewReader(bytes.NewReader(rawCompressed))
		if err != nil {
			return nil, err
		}
		defer gzReader.Close()
		decompReader = gzReader
	default:
		return nil, ErrUnsupportedCodec
	}

	decoder := json.NewDecoder(decompReader)
	var records []ArchiveRecord

	for {
		var rec ArchiveRecord
		if err := decoder.Decode(&rec); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		records = append(records, rec)
	}

	if len(records) != manifest.TotalRecords {
		return nil, ErrInvalidArchive
	}

	return records, nil
}
