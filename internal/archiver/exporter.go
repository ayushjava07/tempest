package archiver

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"time"
)

// Exporter writes records into a compressed archive stream.
type Exporter struct {
	version uint32
}

// NewExporter creates a cold storage exporter.
func NewExporter() *Exporter {
	return &Exporter{version: 1}
}

type countingWriter struct {
	w     io.Writer
	count int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.count += int64(n)
	return n, err
}

// Export streams records as compressed JSON Lines into destination writer.
func (e *Exporter) Export(records []ArchiveRecord, codec Codec, w io.Writer) (*Manifest, error) {
	hasher := sha256.New()
	compressedCounter := &countingWriter{w: io.MultiWriter(w, hasher)}

	var compWriter io.WriteCloser
	switch codec {
	case CodecNone:
		compWriter = &nopWriteCloser{w: compressedCounter}
	case CodecGzip:
		compWriter = gzip.NewWriter(compressedCounter)
	default:
		return nil, ErrUnsupportedCodec
	}

	uncompressedCounter := &countingWriter{w: compWriter}
	encoder := json.NewEncoder(uncompressedCounter)

	for _, rec := range records {
		if err := encoder.Encode(rec); err != nil {
			_ = compWriter.Close()
			return nil, err
		}
	}

	if err := compWriter.Close(); err != nil {
		return nil, err
	}

	manifest := &Manifest{
		Version:           e.version,
		Codec:             codec,
		TotalRecords:      len(records),
		UncompressedBytes: uncompressedCounter.count,
		CompressedBytes:   compressedCounter.count,
		SHA256:            hex.EncodeToString(hasher.Sum(nil)),
		CreatedAt:         time.Now().UTC(),
	}

	return manifest, nil
}

type nopWriteCloser struct {
	w io.Writer
}

func (n *nopWriteCloser) Write(p []byte) (int, error) {
	return n.w.Write(p)
}

func (n *nopWriteCloser) Close() error {
	return nil
}
