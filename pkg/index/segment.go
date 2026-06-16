package index

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	SegmentMagic    = uint32(0x53535441)
	SegmentVersion  = uint32(1)
	FooterSize      = 52
)

type Segment struct {
	mu           sync.RWMutex
	ID           string
	Path         string
	DocCount     int64
	SizeBytes    int64
	Created      time.Time
	inverted     map[string]*InvertedIndex
	docs         map[string]*Document
	fieldValues  map[string]map[string]interface{}
	loaded       bool
}

type SegmentFooter struct {
	Magic      uint32
	Version    uint32
	DocCount   int64
	IndexOffset int64
	IndexSize  int64
	DocOffset   int64
	DocSize    int64
	CRC32      uint32
}

func NewSegment(id string, path string) *Segment {
	return &Segment{
		ID:          id,
		Path:        path,
		inverted:    make(map[string]*InvertedIndex),
		docs:        make(map[string]*Document),
		fieldValues: make(map[string]map[string]interface{}),
		loaded:      false,
	}
}

func (s *Segment) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return nil
	}

	f, err := os.Open(s.Path)
	if err != nil {
		return fmt.Errorf("open segment: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat segment: %w", err)
	}
	s.SizeBytes = stat.Size()

	footerBytes := make([]byte, FooterSize)
	if _, err := f.ReadAt(footerBytes, stat.Size()-FooterSize); err != nil {
		return fmt.Errorf("read footer: %w", err)
	}

	footer := parseFooter(footerBytes)
	if footer.Magic != SegmentMagic {
		return fmt.Errorf("invalid segment magic: %x", footer.Magic)
	}
	s.DocCount = footer.DocCount

	docBytes := make([]byte, footer.DocSize)
	if _, err := f.ReadAt(docBytes, footer.DocOffset); err != nil {
		return fmt.Errorf("read docs: %w", err)
	}

	var docList []*Document
	if err := json.Unmarshal(docBytes, &docList); err != nil {
		return fmt.Errorf("unmarshal docs: %w", err)
	}

	s.docs = make(map[string]*Document, len(docList))
	for _, doc := range docList {
		s.docs[doc.ID] = doc
	}

	return nil
}

func parseFooter(b []byte) SegmentFooter {
	return SegmentFooter{
		Magic:       binary.LittleEndian.Uint32(b[0:4]),
		Version:     binary.LittleEndian.Uint32(b[4:8]),
		DocCount:    int64(binary.LittleEndian.Uint64(b[8:16])),
		DocOffset:   int64(binary.LittleEndian.Uint64(b[16:24])),
		DocSize:     int64(binary.LittleEndian.Uint64(b[24:32])),
		IndexOffset: int64(binary.LittleEndian.Uint64(b[32:40])),
		IndexSize:   int64(binary.LittleEndian.Uint64(b[40:48])),
		CRC32:       binary.LittleEndian.Uint32(b[48:52]),
	}
}

func writeFooter(b []byte, footer SegmentFooter) {
	binary.LittleEndian.PutUint32(b[0:4], footer.Magic)
	binary.LittleEndian.PutUint32(b[4:8], footer.Version)
	binary.LittleEndian.PutUint64(b[8:16], uint64(footer.DocCount))
	binary.LittleEndian.PutUint64(b[16:24], uint64(footer.DocOffset))
	binary.LittleEndian.PutUint64(b[24:32], uint64(footer.DocSize))
	binary.LittleEndian.PutUint64(b[32:40], uint64(footer.IndexOffset))
	binary.LittleEndian.PutUint64(b[40:48], uint64(footer.IndexSize))
	binary.LittleEndian.PutUint32(b[48:52], footer.CRC32)
}

func WriteSegment(dir string, mem *MemIndex) (*Segment, error) {
	docs := mem.AllDocsWithTombstone()
	if len(docs) == 0 {
		return nil, nil
	}

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].ID < docs[j].ID
	})

	id := generateSegmentID()
	path := filepath.Join(dir, fmt.Sprintf("segment_%s.sst", id))

	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("create segment: %w", err)
	}
	defer f.Close()

	docBytes, err := json.Marshal(docs)
	if err != nil {
		return nil, fmt.Errorf("marshal docs: %w", err)
	}

	docOffset, _ := f.Seek(0, 1)
	if _, err := f.Write(docBytes); err != nil {
		return nil, fmt.Errorf("write docs: %w", err)
	}

	indexOffset, _ := f.Seek(0, 1)
	indexSize := int64(0)

	footer := SegmentFooter{
		Magic:      SegmentMagic,
		Version:    SegmentVersion,
		DocCount:   int64(len(docs)),
		DocOffset:  docOffset,
		DocSize:    int64(len(docBytes)),
		IndexOffset: indexOffset,
		IndexSize:  indexSize,
	}

	footerBytes := make([]byte, FooterSize)
	writeFooter(footerBytes, footer)

	crc := crc32.ChecksumIEEE(footerBytes[:48])
	binary.LittleEndian.PutUint32(footerBytes[48:52], crc)
	footer.CRC32 = crc
	writeFooter(footerBytes, footer)

	if _, err := f.Write(footerBytes); err != nil {
		return nil, fmt.Errorf("write footer: %w", err)
	}

	if err := f.Sync(); err != nil {
		return nil, fmt.Errorf("sync segment: %w", err)
	}

	stat, _ := f.Stat()

	seg := NewSegment(id, path)
	seg.DocCount = int64(len(docs))
	seg.SizeBytes = stat.Size()
	seg.Created = time.Now()

	return seg, nil
}

func generateSegmentID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (s *Segment) Get(id string) (*Document, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.loaded {
		if err := s.Load(); err != nil {
			return nil, false
		}
	}
	doc, ok := s.docs[id]
	if ok && doc.Tombstone {
		return nil, false
	}
	return doc, ok
}

func (s *Segment) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inverted = nil
	s.docs = nil
	s.fieldValues = nil
	s.loaded = false
	return nil
}

func (s *Segment) Delete() error {
	if err := s.Close(); err != nil {
		return err
	}
	return os.Remove(s.Path)
}

func (s *Segment) LoadFull() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return nil
	}
	return s.loadFullLocked()
}

func (s *Segment) loadFullLocked() error {
	f, err := os.Open(s.Path)
	if err != nil {
		return fmt.Errorf("open segment: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat segment: %w", err)
	}

	footerBytes := make([]byte, FooterSize)
	if _, err := f.ReadAt(footerBytes, stat.Size()-FooterSize); err != nil {
		return fmt.Errorf("read footer: %w", err)
	}

	footer := parseFooter(footerBytes)
	s.DocCount = footer.DocCount

	docBytes := make([]byte, footer.DocSize)
	if _, err := f.ReadAt(docBytes, footer.DocOffset); err != nil {
		return fmt.Errorf("read docs: %w", err)
	}

	var docList []*Document
	if err := json.Unmarshal(docBytes, &docList); err != nil {
		return fmt.Errorf("unmarshal docs: %w", err)
	}

	s.docs = make(map[string]*Document)
	s.fieldValues = make(map[string]map[string]interface{})
	s.inverted = make(map[string]*InvertedIndex)

	for _, doc := range docList {
		s.docs[doc.ID] = doc
		for field, value := range doc.Fields {
			if _, ok := s.fieldValues[field]; !ok {
				s.fieldValues[field] = make(map[string]interface{})
			}
			s.fieldValues[field][doc.ID] = value
		}
	}

	s.loaded = true
	s.SizeBytes = stat.Size()
	return nil
}

func (s *Segment) GetFieldValue(field string, docID string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.loaded {
		if err := s.loadFullLocked(); err != nil {
			return nil, false
		}
	}
	if values, ok := s.fieldValues[field]; ok {
		v, ok := values[docID]
		return v, ok
	}
	return nil, false
}

func (s *Segment) IsLoaded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loaded
}

func (s *Segment) AllDocs() []*Document {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.loaded {
		if err := s.loadFullLocked(); err != nil {
			return nil
		}
	}
	docs := make([]*Document, 0, len(s.docs))
	for _, doc := range s.docs {
		if !doc.Tombstone {
			docs = append(docs, doc)
		}
	}
	return docs
}

func (s *Segment) AllDocsWithTombstone() []*Document {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.loaded {
		if err := s.loadFullLocked(); err != nil {
			return nil
		}
	}
	docs := make([]*Document, 0, len(s.docs))
	for _, doc := range s.docs {
		docs = append(docs, doc)
	}
	return docs
}
