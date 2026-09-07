package wal

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Entry struct {
	Index     int64     `json:"index"`
	Term      int64     `json:"term"`
	Type      string    `json:"type"`
	Data      []byte    `json:"data"`
	Timestamp time.Time `json:"timestamp"`
}

type WAL struct {
	mu       sync.Mutex
	file     *os.File
	writer   *bufio.Writer
	path     string
	index    int64
	term     int64
	syncEvery int
}

func Open(path string, syncEvery int) (*WAL, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	w := &WAL{
		file:      f,
		writer:    bufio.NewWriter(f),
		path:      path,
		syncEvery: syncEvery,
	}
	if err := w.replay(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *WAL) replay() error {
	scanner := bufio.NewScanner(w.file)
	var maxIndex int64
	for scanner.Scan() {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		if e.Index > maxIndex {
			maxIndex = e.Index
		}
	}
	w.index = maxIndex
	return scanner.Err()
}

func (w *WAL) Append(entry Entry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.index++
	entry.Index = w.index
	entry.Timestamp = time.Now()
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := w.writer.Write(data); err != nil {
		return err
	}
	if err := w.writer.WriteByte('\n'); err != nil {
		return err
	}
	if w.syncEvery > 0 && w.index%int64(w.syncEvery) == 0 {
		if err := w.writer.Flush(); err != nil {
			return err
		}
		if err := w.file.Sync(); err != nil {
			return err
		}
	}
	return nil
}

func (w *WAL) ReadAll() ([]Entry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.readAll()
}

func (w *WAL) readAll() ([]Entry, error) {
	if _, err := w.file.Seek(0, 0); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(w.file)
	var entries []Entry
	for scanner.Scan() {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, scanner.Err()
}

func (w *WAL) LastIndex() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.index
}

func (w *WAL) SetTerm(term int64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.term = term
}

func (w *WAL) Term() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.term
}

func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.writer.Flush(); err != nil {
		return err
	}
	return w.file.Close()
}

func (w *WAL) Truncate(afterIndex int64) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	entries, err := w.readAll()
	if err != nil {
		return err
	}
	var keep []Entry
	for _, e := range entries {
		if e.Index <= afterIndex {
			keep = append(keep, e)
		}
	}
	if err := w.file.Truncate(0); err != nil {
		return err
	}
	if _, err := w.file.Seek(0, 0); err != nil {
		return err
	}
	w.writer = bufio.NewWriter(w.file)
	w.index = 0
	for _, e := range keep {
		data, _ := json.Marshal(e)
		w.writer.Write(data)
		w.writer.WriteByte('\n')
		w.index = e.Index
	}
	return w.writer.Flush()
}