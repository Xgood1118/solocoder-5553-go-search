package index

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type WALOperation string

const (
	OpUpsert WALOperation = "upsert"
	OpDelete WALOperation = "delete"
)

type WALEntry struct {
	Op        WALOperation         `json:"op"`
	DocID     string               `json:"doc_id"`
	Document  *Document            `json:"doc,omitempty"`
	Timestamp time.Time            `json:"ts"`
	Version   int64                `json:"version"`
}

type WAL struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	writer   *bufio.Writer
	entryCount int64
}

func NewWAL(dir string, indexName string) (*WAL, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create wal dir: %w", err)
	}

	path := filepath.Join(dir, fmt.Sprintf("%s.wal", indexName))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("open wal: %w", err)
	}

	wal := &WAL{
		path:   path,
		file:   f,
		writer: bufio.NewWriter(f),
	}

	return wal, nil
}

func (w *WAL) Append(entry *WALEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal wal entry: %w", err)
	}

	if _, err := w.writer.Write(data); err != nil {
		return fmt.Errorf("write wal entry: %w", err)
	}
	if _, err := w.writer.Write([]byte("\n")); err != nil {
		return fmt.Errorf("write wal newline: %w", err)
	}

	w.entryCount++
	return nil
}

func (w *WAL) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := w.writer.Flush(); err != nil {
		return err
	}
	return w.file.Sync()
}

func (w *WAL) Recover() ([]*WALEntry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, err := w.file.Seek(0, 0); err != nil {
		return nil, fmt.Errorf("seek wal: %w", err)
	}

	var entries []*WALEntry
	scanner := bufio.NewScanner(w.file)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry WALEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		entries = append(entries, &entry)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan wal: %w", err)
	}

	w.entryCount = int64(len(entries))
	return entries, nil
}

func (w *WAL) Rotate() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.writer.Flush(); err != nil {
		return err
	}
	if err := w.file.Sync(); err != nil {
		return err
	}
	if err := w.file.Close(); err != nil {
		return err
	}

	backupPath := w.path + fmt.Sprintf(".%d", time.Now().UnixNano())
	if err := os.Rename(w.path, backupPath); err != nil {
		return fmt.Errorf("rotate wal: %w", err)
	}

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("open new wal: %w", err)
	}

	w.file = f
	w.writer = bufio.NewWriter(f)
	w.entryCount = 0

	return nil
}

func (w *WAL) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.writer != nil {
		if err := w.writer.Flush(); err != nil {
			return err
		}
	}
	if w.file != nil {
		if err := w.file.Sync(); err != nil {
			return err
		}
		return w.file.Close()
	}
	return nil
}

func (w *WAL) EntryCount() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.entryCount
}
