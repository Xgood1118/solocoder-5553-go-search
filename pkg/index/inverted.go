package index

import "sort"

type Posting struct {
	DocID    string
	TermFreq int
	Positions []int
}

type PostingList struct {
	Term     string
	Postings []Posting
	DocFreq  int
}

type InvertedIndex struct {
	postings map[string]*PostingList
}

func NewInvertedIndex() *InvertedIndex {
	return &InvertedIndex{
		postings: make(map[string]*PostingList),
	}
}

func (idx *InvertedIndex) Add(term string, docID string, positions []int) {
	term = normalizeTerm(term)
	pl, ok := idx.postings[term]
	if !ok {
		pl = &PostingList{
			Term:     term,
			Postings: make([]Posting, 0),
		}
		idx.postings[term] = pl
	}

	for i, p := range pl.Postings {
		if p.DocID == docID {
			pl.Postings[i].TermFreq = len(positions)
			pl.Postings[i].Positions = positions
			return
		}
	}

	pl.Postings = append(pl.Postings, Posting{
		DocID:    docID,
		TermFreq: len(positions),
		Positions: positions,
	})
	pl.DocFreq++
}

func (idx *InvertedIndex) Get(term string) (*PostingList, bool) {
	term = normalizeTerm(term)
	pl, ok := idx.postings[term]
	return pl, ok
}

func (idx *InvertedIndex) DocFreq(term string) int {
	if pl, ok := idx.Get(term); ok {
		return pl.DocFreq
	}
	return 0
}

func (idx *InvertedIndex) TermCount() int {
	return len(idx.postings)
}

func (idx *InvertedIndex) AllTerms() []string {
	terms := make([]string, 0, len(idx.postings))
	for term := range idx.postings {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	return terms
}

func (idx *InvertedIndex) PrefixTerms(prefix string) []string {
	var terms []string
	for term := range idx.postings {
		if len(term) >= len(prefix) && term[:len(prefix)] == prefix {
			terms = append(terms, term)
		}
	}
	sort.Strings(terms)
	return terms
}

func (idx *InvertedIndex) RemoveDoc(docID string) {
	for term, pl := range idx.postings {
		for i, p := range pl.Postings {
			if p.DocID == docID {
				pl.Postings = append(pl.Postings[:i], pl.Postings[i+1:]...)
				pl.DocFreq--
				if pl.DocFreq == 0 {
					delete(idx.postings, term)
				}
				break
			}
		}
	}
}

func normalizeTerm(term string) string {
	return term
}

func IntersectPostings(lists []*PostingList) []Posting {
	if len(lists) == 0 {
		return nil
	}
	if len(lists) == 1 {
		return lists[0].Postings
	}

	docMap := make(map[string]Posting)
	for _, p := range lists[0].Postings {
		docMap[p.DocID] = p
	}

	for i := 1; i < len(lists); i++ {
		newMap := make(map[string]Posting)
		for _, p := range lists[i].Postings {
			if existing, ok := docMap[p.DocID]; ok {
				merged := Posting{
					DocID:    p.DocID,
					TermFreq: existing.TermFreq + p.TermFreq,
					Positions: append(existing.Positions, p.Positions...),
				}
				newMap[p.DocID] = merged
			}
		}
		docMap = newMap
		if len(docMap) == 0 {
			return nil
		}
	}

	result := make([]Posting, 0, len(docMap))
	for _, p := range docMap {
		result = append(result, p)
	}
	return result
}

func UnionPostings(lists []*PostingList) []Posting {
	if len(lists) == 0 {
		return nil
	}

	docMap := make(map[string]Posting)
	for _, list := range lists {
		for _, p := range list.Postings {
			if existing, ok := docMap[p.DocID]; ok {
				merged := Posting{
					DocID:    p.DocID,
					TermFreq: existing.TermFreq + p.TermFreq,
					Positions: append(existing.Positions, p.Positions...),
				}
				docMap[p.DocID] = merged
			} else {
				docMap[p.DocID] = p
			}
		}
	}

	result := make([]Posting, 0, len(docMap))
	for _, p := range docMap {
		result = append(result, p)
	}
	return result
}
