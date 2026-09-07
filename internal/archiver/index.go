package archiver

import (
	"encoding/json"
	"io"
)

// IndexEntry references a specific record location.
type IndexEntry struct {
	RunID      string `json:"run_id"`
	WorkflowID string `json:"workflow_id"`
	Index      int    `json:"index"`
}

// Index maps run IDs to their index entry in the archive.
type Index struct {
	Entries map[string]IndexEntry `json:"entries"`
}

// BuildIndex constructs a fast lookup index from an archive record slice.
func BuildIndex(records []ArchiveRecord) *Index {
	idx := &Index{
		Entries: make(map[string]IndexEntry, len(records)),
	}

	for i, r := range records {
		idx.Entries[r.RunID] = IndexEntry{
			RunID:      r.RunID,
			WorkflowID: r.WorkflowID,
			Index:      i,
		}
	}

	return idx
}

// WriteIndex serializes index to JSON writer.
func (idx *Index) Write(w io.Writer) error {
	return json.NewEncoder(w).Encode(idx)
}

// ReadIndex deserializes index from JSON reader.
func ReadIndex(r io.Reader) (*Index, error) {
	var idx Index
	if err := json.NewDecoder(r).Decode(&idx); err != nil {
		return nil, err
	}
	return &idx, nil
}
