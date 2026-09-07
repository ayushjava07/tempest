package archiver

import (
	"errors"
	"time"
)

var (
	ErrInvalidArchive   = errors.New("archiver: invalid or malformed archive format")
	ErrChecksumMismatch = errors.New("archiver: manifest checksum mismatch")
	ErrUnsupportedCodec = errors.New("archiver: unsupported compression codec")
	ErrRunNotFound      = errors.New("archiver: workflow run not found in archive")
)

// Codec identifies compression algorithm.
type Codec string

const (
	CodecNone Codec = "none"
	CodecGzip Codec = "gzip"
)

// StepArchive captures final step outcome metadata.
type StepArchive struct {
	StepID    string            `json:"step_id"`
	State     string            `json:"state"`
	ExitCode  int               `json:"exit_code"`
	StartedAt time.Time         `json:"started_at"`
	EndedAt   time.Time         `json:"ended_at"`
	Output    map[string]string `json:"output,omitempty"`
}

// ArchiveRecord captures complete workflow execution state for cold storage.
type ArchiveRecord struct {
	RunID       string            `json:"run_id"`
	WorkflowID  string            `json:"workflow_id"`
	Namespace   string            `json:"namespace"`
	FinalState  string            `json:"final_state"`
	StartedAt   time.Time         `json:"started_at"`
	FinishedAt  time.Time         `json:"finished_at"`
	Variables   map[string]string `json:"variables,omitempty"`
	Steps       []StepArchive     `json:"steps"`
	PayloadJSON []byte            `json:"payload_json,omitempty"`
}

// Manifest provides verification metadata and integrity digests for an archive file.
type Manifest struct {
	Version           uint32    `json:"version"`
	Codec             Codec     `json:"codec"`
	TotalRecords      int       `json:"total_records"`
	UncompressedBytes int64     `json:"uncompressed_bytes"`
	CompressedBytes   int64     `json:"compressed_bytes"`
	SHA256            string    `json:"sha256"`
	CreatedAt         time.Time `json:"created_at"`
}
