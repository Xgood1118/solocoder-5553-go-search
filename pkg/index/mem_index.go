package index

import (
	"sync"
	"time"
)

type MemIndex struct {
	mu          sync.RWMutex
	docs        map[string]*Document
	inverted    map[string]*InvertedIndex
	fieldValues map[string]map[string]interface{}
	docCount    int64
	tombstoneCount int64
	sizeBytes   int64
}

func NewMemIndex() *MemIndex {
	return &MemIndex{
		docs:        make(map[string]*Document),
		inverted:    make(map[string]*InvertedIndex),
		fieldValues: make(map[string]map[string]interface{}),
	}
}

func (m *MemIndex) Add(doc *Document, fieldTokens map[string][]TokenPos) {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.docs[doc.ID]
	if exists {
		if existing.Tombstone {
			m.tombstoneCount--
		} else {
			m.docCount--
		}
		for field := range m.inverted {
			m.inverted[field].RemoveDoc(doc.ID)
		}
	}

	m.docs[doc.ID] = doc
	if doc.Tombstone {
		m.tombstoneCount++
	} else {
		m.docCount++
	}

	for field, tokens := range fieldTokens {
		if _, ok := m.inverted[field]; !ok {
			m.inverted[field] = NewInvertedIndex()
		}
		positions := make([]int, len(tokens))
		for i, t := range tokens {
			positions[i] = t.Position
			m.sizeBytes += int64(len(t.Term))
		}
		m.inverted[field].Add(field, doc.ID, positions)
		for _, t := range tokens {
			m.inverted[field].Add(t.Term, doc.ID, positions)
		}
	}

	for field, value := range doc.Fields {
		if _, ok := m.fieldValues[field]; !ok {
			m.fieldValues[field] = make(map[string]interface{})
		}
		m.fieldValues[field][doc.ID] = value
	}
}

func (m *MemIndex) Get(id string) (*Document, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	doc, ok := m.docs[id]
	if ok && doc.Tombstone {
		return nil, false
	}
	return doc, ok
}

func (m *MemIndex) GetWithTombstone(id string) (*Document, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	doc, ok := m.docs[id]
	return doc, ok
}

func (m *MemIndex) Delete(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if doc, ok := m.docs[id]; ok {
		if !doc.Tombstone {
			doc.Tombstone = true
			doc.UpdatedAt = time.Now()
			m.docCount--
			m.tombstoneCount++
			return true
		}
	}
	m.docs[id] = &Document{
		ID:        id,
		Fields:    make(map[string]interface{}),
		Tombstone: true,
		Version:   1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	m.tombstoneCount++
	return true
}

func (m *MemIndex) DocCount() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.docCount
}

func (m *MemIndex) TombstoneCount() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tombstoneCount
}

func (m *MemIndex) SizeBytes() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sizeBytes
}

func (m *MemIndex) GetPostingList(field string, term string) (*PostingList, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if idx, ok := m.inverted[field]; ok {
		return idx.Get(term)
	}
	return nil, false
}

func (m *MemIndex) GetFieldInverted(field string) (*InvertedIndex, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	idx, ok := m.inverted[field]
	return idx, ok
}

func (m *MemIndex) GetFieldValue(field string, docID string) (interface{}, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if values, ok := m.fieldValues[field]; ok {
		v, ok := values[docID]
		return v, ok
	}
	return nil, false
}

func (m *MemIndex) AllDocs() []*Document {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*Document, 0, len(m.docs))
	for _, doc := range m.docs {
		if !doc.Tombstone {
			result = append(result, doc)
		}
	}
	return result
}

func (m *MemIndex) AllDocsWithTombstone() []*Document {
	m.mu.RLock()
	defer m.mu.RUnlock()
	docs := make([]*Document, 0, len(m.docs))
	for _, doc := range m.docs {
		docs = append(docs, doc)
	}
	return docs
}

func (m *MemIndex) PrefixTerms(field string, prefix string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if idx, ok := m.inverted[field]; ok {
		return idx.PrefixTerms(prefix)
	}
	return nil
}

type TokenPos struct {
	Term     string
	Position int
	Start    int
	End      int
}
