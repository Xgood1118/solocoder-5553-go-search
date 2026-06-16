package index

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/solo/fulltext-search/pkg/analyzer"
	"github.com/solo/fulltext-search/pkg/config"
)

type IndexManager struct {
	mu          sync.RWMutex
	indexes     map[string]*Index
	config      *config.Config
	analyzers   analyzer.Registry
	dataDir     string
}

type Index struct {
	mu          sync.RWMutex
	name        string
	meta        *IndexMeta
	memIndex    *MemIndex
	segments    []*Segment
	wal         *WAL
	analyzer    analyzer.Analyzer
	config      *config.Config
	flushTicker *time.Ticker
	stopChan    chan struct{}
	closed      bool
}

func NewIndexManager(cfg *config.Config, reg analyzer.Registry) *IndexManager {
	im := &IndexManager{
		indexes:   make(map[string]*Index),
		config:    cfg,
		analyzers: reg,
		dataDir:   cfg.DataDir,
	}
	im.loadExistingIndexes()
	return im
}

func (m *IndexManager) loadExistingIndexes() {
	if _, err := os.Stat(m.dataDir); os.IsNotExist(err) {
		return
	}

	entries, err := os.ReadDir(m.dataDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		metaPath := filepath.Join(m.dataDir, name, "meta.json")
		if _, err := os.Stat(metaPath); os.IsNotExist(err) {
			continue
		}
		if err := m.loadIndex(name); err != nil {
			fmt.Printf("warn: failed to load index %s: %v\n", name, err)
		}
	}
}

func (m *IndexManager) loadIndex(name string) error {
	idxDir := filepath.Join(m.dataDir, name)
	metaPath := filepath.Join(idxDir, "meta.json")

	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		return fmt.Errorf("read meta: %w", err)
	}

	var meta IndexMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return fmt.Errorf("unmarshal meta: %w", err)
	}

	analyzerName := m.config.Analyzer.Default
	if meta.Settings.Analyzer != "" {
		analyzerName = meta.Settings.Analyzer
	}
	a, _ := m.analyzers.Get(analyzerName)
	if a == nil {
		a = m.analyzers.Default()
	}

	idx := &Index{
		name:     name,
		meta:     &meta,
		memIndex: NewMemIndex(),
		segments: make([]*Segment, 0),
		analyzer: a,
		config:   m.config,
		stopChan: make(chan struct{}),
	}

	wal, err := NewWAL(idxDir, name)
	if err != nil {
		return fmt.Errorf("create wal: %w", err)
	}
	idx.wal = wal

	if err := idx.loadSegments(); err != nil {
		return fmt.Errorf("load segments: %w", err)
	}

	if err := idx.recoverFromWAL(); err != nil {
		return fmt.Errorf("recover from wal: %w", err)
	}

	m.indexes[name] = idx
	idx.startBackgroundFlush()

	return nil
}

func (m *IndexManager) CreateIndex(name string, mapping *IndexMapping, settings *IndexSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.indexes[name]; ok {
		return fmt.Errorf("index %s already exists", name)
	}

	if settings == nil {
		settings = &IndexSettings{}
	}
	if settings.NumberOfShards == 0 {
		settings.NumberOfShards = m.config.Index.Shards
	}
	if mapping == nil {
		mapping = &IndexMapping{Properties: make(map[string]FieldMapping)}
	}

	analyzerName := m.config.Analyzer.Default
	if settings.Analyzer != "" {
		analyzerName = settings.Analyzer
	}
	a, _ := m.analyzers.Get(analyzerName)
	if a == nil {
		a = m.analyzers.Default()
	}

	idxDir := filepath.Join(m.dataDir, name)
	if err := os.MkdirAll(idxDir, 0755); err != nil {
		return fmt.Errorf("create index dir: %w", err)
	}

	idx := &Index{
		name:     name,
		memIndex: NewMemIndex(),
		segments: make([]*Segment, 0),
		analyzer: a,
		config:   m.config,
		meta: &IndexMeta{
			Name:     name,
			Mapping:  *mapping,
			Settings: *settings,
			Created:  time.Now(),
			Updated:  time.Now(),
		},
		stopChan: make(chan struct{}),
	}

	wal, err := NewWAL(idxDir, name)
	if err != nil {
		return fmt.Errorf("create wal: %w", err)
	}
	idx.wal = wal

	if err := idx.loadSegments(); err != nil {
		return fmt.Errorf("load segments: %w", err)
	}

	if err := idx.recoverFromWAL(); err != nil {
		return fmt.Errorf("recover from wal: %w", err)
	}

	m.indexes[name] = idx
	idx.startBackgroundFlush()

	if err := idx.saveMeta(); err != nil {
		return fmt.Errorf("save meta: %w", err)
	}

	return nil
}

func (m *IndexManager) DeleteIndex(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx, ok := m.indexes[name]
	if !ok {
		return fmt.Errorf("index %s not found", name)
	}

	idx.close()

	idxDir := filepath.Join(m.dataDir, name)
	if err := os.RemoveAll(idxDir); err != nil {
		return fmt.Errorf("remove index dir: %w", err)
	}

	delete(m.indexes, name)
	return nil
}

func (m *IndexManager) GetIndex(name string) (*Index, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	idx, ok := m.indexes[name]
	return idx, ok
}

func (m *IndexManager) ListIndexes() []*IndexMeta {
	m.mu.RLock()
	defer m.mu.RUnlock()
	metas := make([]*IndexMeta, 0, len(m.indexes))
	for _, idx := range m.indexes {
		meta := *idx.meta
		meta.DocCount = idx.DocCount()
		metas = append(metas, &meta)
	}
	return metas
}

func (m *IndexManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, idx := range m.indexes {
		idx.close()
	}
	m.indexes = make(map[string]*Index)
}

func (idx *Index) Name() string {
	return idx.name
}

func (idx *Index) Meta() *IndexMeta {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	meta := *idx.meta
	meta.DocCount = idx.DocCount()
	return &meta
}

func (idx *Index) DocCount() int64 {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	count := idx.memIndex.DocCount()
	for _, seg := range idx.segments {
		// 粗略统计，不加载完整段
		count += seg.DocCount
	}
	return count
}

func (idx *Index) AddDocument(doc *Document) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return fmt.Errorf("index is closed")
	}

	existing, _ := idx.memIndex.GetWithTombstone(doc.ID)
	version := int64(1)
	if existing != nil {
		version = existing.Version + 1
	}
	doc.Version = version
	doc.UpdatedAt = time.Now()
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now()
	}

	fieldTokens := idx.analyzeDocument(doc)

	entry := &WALEntry{
		Op:        OpUpsert,
		DocID:     doc.ID,
		Document:  doc,
		Timestamp: time.Now(),
		Version:   version,
	}
	if err := idx.wal.Append(entry); err != nil {
		return fmt.Errorf("append wal: %w", err)
	}

	idx.memIndex.Add(doc, fieldTokens)
	idx.meta.Updated = time.Now()

	return nil
}

func (idx *Index) GetDocument(id string) (*Document, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if doc, ok := idx.memIndex.Get(id); ok {
		return doc, true
	}

	for _, seg := range idx.segments {
		if doc, ok := seg.Get(id); ok {
			return doc, true
		}
	}

	return nil, false
}

func (idx *Index) DeleteDocument(id string) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return fmt.Errorf("index is closed")
	}

	entry := &WALEntry{
		Op:        OpDelete,
		DocID:     id,
		Timestamp: time.Now(),
	}
	if err := idx.wal.Append(entry); err != nil {
		return fmt.Errorf("append wal: %w", err)
	}

	idx.memIndex.Delete(id)
	idx.meta.Updated = time.Now()

	return nil
}

func (idx *Index) analyzeDocument(doc *Document) map[string][]TokenPos {
	result := make(map[string][]TokenPos)

	for field, value := range doc.Fields {
		text := fmt.Sprintf("%v", value)
		tokens := idx.analyzer.Analyze(text)
		tokenPos := make([]TokenPos, len(tokens))
		for i, t := range tokens {
			tokenPos[i] = TokenPos{
				Term:     t.Term,
				Position: t.Position,
				Start:    t.Start,
				End:      t.End,
			}
		}
		result[field] = tokenPos
	}

	return result
}

func (idx *Index) startBackgroundFlush() {
	idx.flushTicker = time.NewTicker(time.Duration(idx.config.Index.FlushSeconds) * time.Second)
	go func() {
		for {
			select {
			case <-idx.flushTicker.C:
				idx.Flush()
			case <-idx.stopChan:
				return
			}
		}
	}()
}

func (idx *Index) Flush() error {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	return idx.flushLocked()
}

func (idx *Index) flushLocked() error {
	if idx.memIndex.DocCount() == 0 && idx.memIndex.TombstoneCount() == 0 {
		return nil
	}

	idxDir := filepath.Join(idx.config.DataDir, idx.name)
	seg, err := WriteSegment(idxDir, idx.memIndex)
	if err != nil {
		return fmt.Errorf("write segment: %w", err)
	}

	if seg != nil {
		idx.segments = append(idx.segments, seg)
		idx.memIndex = NewMemIndex()

		if err := idx.wal.Rotate(); err != nil {
			return fmt.Errorf("rotate wal: %w", err)
		}

		go idx.checkMerge()
	}

	return nil
}

func (idx *Index) checkMerge() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if len(idx.segments) < idx.config.Index.MergeFactor {
		return
	}

	if err := idx.mergeLocked(); err != nil {
		fmt.Printf("merge error: %v\n", err)
	}
}

func (idx *Index) mergeLocked() error {
	if len(idx.segments) < 2 {
		return nil
	}

	mergeCount := min(len(idx.segments), idx.config.Index.MergeFactor)
	toMerge := idx.segments[:mergeCount]

	mergedMem := NewMemIndex()
	deletedDocs := make(map[string]bool)

	for i := len(toMerge) - 1; i >= 0; i-- {
		seg := toMerge[i]
		if err := seg.LoadFull(); err != nil {
			return fmt.Errorf("load segment for merge: %w", err)
		}

		docs := seg.AllDocsWithTombstone()
		for _, doc := range docs {
			if deletedDocs[doc.ID] {
				continue
			}
			if doc.Tombstone {
				deletedDocs[doc.ID] = true
				mergedMem.Delete(doc.ID)
			} else {
				fieldTokens := idx.analyzeDocument(doc)
				mergedMem.Add(doc, fieldTokens)
				deletedDocs[doc.ID] = true
			}
		}
	}

	idxDir := filepath.Join(idx.config.DataDir, idx.name)
	mergedSeg, err := WriteSegment(idxDir, mergedMem)
	if err != nil {
		return fmt.Errorf("write merged segment: %w", err)
	}

	if mergedSeg != nil {
		for _, seg := range toMerge {
			seg.Delete()
		}

		remaining := idx.segments[mergeCount:]
		idx.segments = append([]*Segment{mergedSeg}, remaining...)
	}

	return nil
}



func (idx *Index) ForceMerge(maxSegments int) error {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if maxSegments <= 0 {
		maxSegments = 1
	}

	for len(idx.segments) > maxSegments {
		if err := idx.mergeLocked(); err != nil {
			return err
		}
	}
	return nil
}

func (idx *Index) Refresh() error {
	return idx.Flush()
}

func (idx *Index) close() {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	if idx.closed {
		return
	}
	idx.closed = true

	if idx.flushTicker != nil {
		idx.flushTicker.Stop()
	}
	close(idx.stopChan)

	idx.flushLocked()

	if idx.wal != nil {
		idx.wal.Close()
	}

	for _, seg := range idx.segments {
		seg.Close()
	}
}

func (idx *Index) loadSegments() error {
	idxDir := filepath.Join(idx.config.DataDir, idx.name)
	matches, err := filepath.Glob(filepath.Join(idxDir, "segment_*.sst"))
	if err != nil {
		return err
	}

	sort.Strings(matches)

	for _, path := range matches {
		id := filepath.Base(path)
		id = id[len("segment_") : len(id)-len(".sst")]
		seg := NewSegment(id, path)
		idx.segments = append(idx.segments, seg)

		if err := seg.LoadFull(); err != nil {
			return fmt.Errorf("load segment %s: %w", id, err)
		}
	}

	if err := idx.loadSegmentsToMemory(); err != nil {
		return fmt.Errorf("load segments to memory: %w", err)
	}

	return nil
}

func (idx *Index) loadSegmentsToMemory() error {
	deletedDocs := make(map[string]bool)

	for i := len(idx.segments) - 1; i >= 0; i-- {
		seg := idx.segments[i]
		docs := seg.AllDocs()
		for _, doc := range docs {
			if deletedDocs[doc.ID] {
				continue
			}
			if doc.Tombstone {
				deletedDocs[doc.ID] = true
				continue
			}
			fieldTokens := idx.analyzeDocument(doc)
			idx.memIndex.Add(doc, fieldTokens)
			deletedDocs[doc.ID] = true
		}
	}

	return nil
}

func (idx *Index) recoverFromWAL() error {
	entries, err := idx.wal.Recover()
	if err != nil {
		return err
	}

	for _, entry := range entries {
		switch entry.Op {
		case OpUpsert:
			if entry.Document != nil {
				fieldTokens := idx.analyzeDocument(entry.Document)
				idx.memIndex.Add(entry.Document, fieldTokens)
			}
		case OpDelete:
			idx.memIndex.Delete(entry.DocID)
		}
	}

	return nil
}

func (idx *Index) saveMeta() error {
	idxDir := filepath.Join(idx.config.DataDir, idx.name)
	metaPath := filepath.Join(idxDir, "meta.json")

	data, err := json.MarshalIndent(idx.meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, 0644)
}

func (idx *Index) GetAnalyzer() analyzer.Analyzer {
	return idx.analyzer
}

func (idx *Index) MemIndex() *MemIndex {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.memIndex
}

func (idx *Index) Segments() []*Segment {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.segments
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
