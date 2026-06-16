package query

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/solo/fulltext-search/pkg/index"
)

type SearchResult struct {
	Total        int64
	MaxScore     float64
	Hits         []*SearchHit
	Aggregations map[string]AggregationResult
}

type SearchHit struct {
	ID          string
	Score       float64
	Source      map[string]interface{}
	Highlight   map[string][]string
	SortValues  []interface{}
}

type AggregationResult struct {
	Name    string
	Type    string
	Buckets []AggBucket
}

type AggBucket struct {
	Key         interface{}
	KeyAsString string
	DocCount    int64
}

type Searcher struct {
	idx *index.Index
}

func NewSearcher(idx *index.Index) *Searcher {
	return &Searcher{idx: idx}
}

func (s *Searcher) Search(req *SearchRequest) (*SearchResult, error) {
	scorer := NewBM25Scorer(s.idx)

	docScores := make(map[string]float64)
	matchedDocs := make(map[string]*index.Document)

	s.collectMatches(req.Query, docScores, matchedDocs, scorer)

	if len(matchedDocs) == 0 {
		return &SearchResult{Total: 0, Hits: []*SearchHit{}}, nil
	}

	hits := make([]*SearchHit, 0, len(matchedDocs))
	for docID, doc := range matchedDocs {
		score := docScores[docID]
		hits = append(hits, &SearchHit{
			ID:     docID,
			Score:  score,
			Source: doc.Fields,
		})
	}

	sort.Slice(hits, func(i, j int) bool {
		if len(req.Sort) == 0 {
			return hits[i].Score > hits[j].Score
		}
		return s.compareSort(hits[i], hits[j], req.Sort, matchedDocs)
	})

	if req.SearchAfter != nil && len(req.SearchAfter) > 0 {
		hits = s.applySearchAfter(hits, req.SearchAfter, req.Sort, matchedDocs)
	}

	total := int64(len(hits))

	start := req.From
	end := start + req.Size
	if start > len(hits) {
		start = len(hits)
	}
	if end > len(hits) {
		end = len(hits)
	}
	pagedHits := hits[start:end]

	maxScore := 0.0
	for _, h := range pagedHits {
		if h.Score > maxScore {
			maxScore = h.Score
		}
	}

	if req.Highlight != nil {
		s.applyHighlight(pagedHits, req.Highlight, req.Query)
	}

	if len(req.Aggregations) > 0 {
		aggResults := s.computeAggregations(hits, req.Aggregations)
		return &SearchResult{
			Total:        total,
			MaxScore:     maxScore,
			Hits:         pagedHits,
			Aggregations: aggResults,
		}, nil
	}

	if req.Source != nil {
		for _, hit := range pagedHits {
			filtered := make(map[string]interface{})
			for _, field := range req.Source {
				if v, ok := hit.Source[field]; ok {
					filtered[field] = v
				}
			}
			hit.Source = filtered
		}
	}

	for _, hit := range pagedHits {
		hit.SortValues = s.getSortValues(hit, req.Sort, matchedDocs)
	}

	return &SearchResult{
		Total:    total,
		MaxScore: maxScore,
		Hits:     pagedHits,
	}, nil
}

func (s *Searcher) collectMatches(q Query, scores map[string]float64, docs map[string]*index.Document, scorer *BM25Scorer) {
	switch query := q.(type) {
	case *MatchAllQuery:
		allDocs := s.idx.MemIndex().AllDocs()
		for _, doc := range allDocs {
			docs[doc.ID] = doc
			scores[doc.ID] = 1.0
		}
	case *TermQuery:
		pl, ok := s.idx.MemIndex().GetPostingList(query.Field, query.Value)
		if ok {
			for _, p := range pl.Postings {
				doc, exists := s.idx.GetDocument(p.DocID)
				if exists {
					docs[p.DocID] = doc
					scores[p.DocID] = scorer.Score(query.Field, query.Value, p.TermFreq)
				}
			}
		}
	case *TermsQuery:
		for _, value := range query.Values {
			pl, ok := s.idx.MemIndex().GetPostingList(query.Field, value)
			if ok {
				for _, p := range pl.Postings {
					doc, exists := s.idx.GetDocument(p.DocID)
					if exists {
						docs[p.DocID] = doc
						scores[p.DocID] += scorer.Score(query.Field, value, p.TermFreq)
					}
				}
			}
		}
	case *MatchQuery:
		tokens := s.idx.GetAnalyzer().Analyze(query.Query)
		for _, token := range tokens {
			pl, ok := s.idx.MemIndex().GetPostingList(query.Field, token.Term)
			if ok {
				for _, p := range pl.Postings {
					doc, exists := s.idx.GetDocument(p.DocID)
					if exists {
						docs[p.DocID] = doc
						scores[p.DocID] += scorer.Score(query.Field, token.Term, p.TermFreq)
					}
				}
			}
		}
	case *RangeQuery:
		allDocs := s.idx.MemIndex().AllDocs()
		for _, doc := range allDocs {
			if value, ok := doc.Fields[query.Field]; ok {
				if s.matchRange(value, query) {
					docs[doc.ID] = doc
					scores[doc.ID] += 1.0
				}
			}
		}
	case *ExistsQuery:
		allDocs := s.idx.MemIndex().AllDocs()
		for _, doc := range allDocs {
			if _, ok := doc.Fields[query.Field]; ok {
				docs[doc.ID] = doc
				scores[doc.ID] += 1.0
			}
		}
	case *PrefixQuery:
		inv, ok := s.idx.MemIndex().GetFieldInverted(query.Field)
		if ok {
			terms := inv.PrefixTerms(query.Value)
			for _, term := range terms {
				pl, _ := inv.Get(term)
				if pl != nil {
					for _, p := range pl.Postings {
						doc, exists := s.idx.GetDocument(p.DocID)
						if exists {
							docs[p.DocID] = doc
							scores[p.DocID] += scorer.Score(query.Field, term, p.TermFreq)
						}
					}
				}
			}
		}
	case *WildcardQuery:
		inv, ok := s.idx.MemIndex().GetFieldInverted(query.Field)
		if ok {
			pattern := strings.ToLower(query.Value)
			for _, term := range inv.AllTerms() {
				if matchWildcard(term, pattern) {
					pl, _ := inv.Get(term)
					if pl != nil {
						for _, p := range pl.Postings {
							doc, exists := s.idx.GetDocument(p.DocID)
							if exists {
								docs[p.DocID] = doc
								scores[p.DocID] += scorer.Score(query.Field, term, p.TermFreq)
							}
						}
					}
				}
			}
		}
	case *RegexQuery:
		inv, ok := s.idx.MemIndex().GetFieldInverted(query.Field)
		if ok {
			for _, term := range inv.AllTerms() {
				if matchSimpleRegex(term, query.Value) {
					pl, _ := inv.Get(term)
					if pl != nil {
						for _, p := range pl.Postings {
							doc, exists := s.idx.GetDocument(p.DocID)
							if exists {
								docs[p.DocID] = doc
								scores[p.DocID] += scorer.Score(query.Field, term, p.TermFreq)
							}
						}
					}
				}
			}
		}
	case *FuzzyQuery:
		inv, ok := s.idx.MemIndex().GetFieldInverted(query.Field)
		if ok {
			for _, term := range inv.AllTerms() {
				if len(term) >= query.PrefixLen && len(query.Value) >= query.PrefixLen {
					if term[:query.PrefixLen] == query.Value[:query.PrefixLen] {
						dist := editDistance(term, query.Value)
						if dist <= query.Fuzziness {
							pl, _ := inv.Get(term)
							if pl != nil {
								for _, p := range pl.Postings {
									doc, exists := s.idx.GetDocument(p.DocID)
									if exists {
										docs[p.DocID] = doc
										scores[p.DocID] += scorer.Score(query.Field, term, p.TermFreq) * (1.0 - float64(dist)/float64(query.Fuzziness+1))
									}
								}
							}
						}
					}
				}
			}
		}
	case *BoolQuery:
		s.executeBoolQuery(query, scores, docs, scorer)
	}
}

func (s *Searcher) executeBoolQuery(q *BoolQuery, scores map[string]float64, docs map[string]*index.Document, scorer *BM25Scorer) {
	mustDocs := make(map[string]*index.Document)
	mustScores := make(map[string]float64)
	firstMust := true

	for _, subQ := range q.Must {
		subDocs := make(map[string]*index.Document)
		subScores := make(map[string]float64)
		s.collectMatches(subQ, subScores, subDocs, scorer)

		if firstMust {
			for id, doc := range subDocs {
				mustDocs[id] = doc
				mustScores[id] = subScores[id]
			}
			firstMust = false
		} else {
			for id := range mustDocs {
				if _, ok := subDocs[id]; ok {
					mustScores[id] += subScores[id]
				} else {
					delete(mustDocs, id)
					delete(mustScores, id)
				}
			}
		}
	}

	filterDocs := make(map[string]*index.Document)
	firstFilter := true
	if len(q.Filter) > 0 {
		for _, subQ := range q.Filter {
			subDocs := make(map[string]*index.Document)
			subScores := make(map[string]float64)
			s.collectMatches(subQ, subScores, subDocs, scorer)

			if firstFilter {
				for id, doc := range subDocs {
					filterDocs[id] = doc
				}
				firstFilter = false
			} else {
				for id := range filterDocs {
					if _, ok := subDocs[id]; !ok {
						delete(filterDocs, id)
					}
				}
			}
		}
	}

	shouldDocs := make(map[string]*index.Document)
	shouldScores := make(map[string]float64)
	for _, subQ := range q.Should {
		subDocs := make(map[string]*index.Document)
		subScores := make(map[string]float64)
		s.collectMatches(subQ, subScores, subDocs, scorer)

		for id, doc := range subDocs {
			shouldDocs[id] = doc
			shouldScores[id] += subScores[id]
		}
	}

	mustNotDocs := make(map[string]*index.Document)
	for _, subQ := range q.MustNot {
		subDocs := make(map[string]*index.Document)
		subScores := make(map[string]float64)
		s.collectMatches(subQ, subScores, subDocs, scorer)
		for id, doc := range subDocs {
			mustNotDocs[id] = doc
		}
	}

	if len(q.Must) > 0 {
		for id := range mustDocs {
			if _, exclude := mustNotDocs[id]; exclude {
				continue
			}
			if len(q.Filter) > 0 {
				if _, inFilter := filterDocs[id]; !inFilter {
					continue
				}
			}
			docs[id] = mustDocs[id]
			scores[id] = mustScores[id]
			if s, ok := shouldScores[id]; ok {
				scores[id] += s
			}
		}
	} else if len(q.Filter) > 0 {
		for id := range filterDocs {
			if _, exclude := mustNotDocs[id]; exclude {
				continue
			}
			docs[id] = filterDocs[id]
			scores[id] = 1.0
			if s, ok := shouldScores[id]; ok {
				scores[id] += s
			}
		}
	} else if len(q.Should) > 0 {
		for id := range shouldDocs {
			if _, exclude := mustNotDocs[id]; exclude {
				continue
			}
			docs[id] = shouldDocs[id]
			scores[id] = shouldScores[id]
		}
	}
}

func (s *Searcher) matchRange(value interface{}, q *RangeQuery) bool {
	switch v := value.(type) {
	case float64:
		return s.matchNumericRange(float64(v), q)
	case int:
		return s.matchNumericRange(float64(v), q)
	case int64:
		return s.matchNumericRange(float64(v), q)
	case string:
		return s.matchStringRange(v, q)
	default:
		return false
	}
}

func (s *Searcher) matchNumericRange(v float64, q *RangeQuery) bool {
	if q.Gte != nil {
		gte := toFloat(q.Gte)
		if v < gte {
			return false
		}
	}
	if q.Gt != nil {
		gt := toFloat(q.Gt)
		if v <= gt {
			return false
		}
	}
	if q.Lte != nil {
		lte := toFloat(q.Lte)
		if v > lte {
			return false
		}
	}
	if q.Lt != nil {
		lt := toFloat(q.Lt)
		if v >= lt {
			return false
		}
	}
	return true
}

func (s *Searcher) matchStringRange(v string, q *RangeQuery) bool {
	if q.Gte != nil {
		if v < fmt.Sprintf("%v", q.Gte) {
			return false
		}
	}
	if q.Gt != nil {
		if v <= fmt.Sprintf("%v", q.Gt) {
			return false
		}
	}
	if q.Lte != nil {
		if v > fmt.Sprintf("%v", q.Lte) {
			return false
		}
	}
	if q.Lt != nil {
		if v >= fmt.Sprintf("%v", q.Lt) {
			return false
		}
	}
	return true
}

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		var f float64
		fmt.Sscanf(val, "%f", &f)
		return f
	default:
		return 0
	}
}

func matchWildcard(str, pattern string) bool {
	str = strings.ToLower(str)
	pattern = strings.ToLower(pattern)
	
	if pattern == "*" {
		return true
	}
	
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return str == pattern
	}
	
	if !strings.HasPrefix(pattern, "*") {
		if !strings.HasPrefix(str, parts[0]) {
			return false
		}
	}
	if !strings.HasSuffix(pattern, "*") {
		if !strings.HasSuffix(str, parts[len(parts)-1]) {
			return false
		}
	}
	
	pos := 0
	for _, part := range parts {
		if part == "" {
			continue
		}
		idx := strings.Index(str[pos:], part)
		if idx == -1 {
			return false
		}
		pos += idx + len(part)
	}
	return true
}

func matchSimpleRegex(str, pattern string) bool {
	str = strings.ToLower(str)
	pattern = strings.ToLower(pattern)
	
	if len(pattern) == 0 {
		return len(str) == 0
	}
	
	if strings.HasPrefix(pattern, "^") {
		pattern = pattern[1:]
	}
	if strings.HasSuffix(pattern, "$") {
		pattern = pattern[:len(pattern)-1]
	}
	
	return strings.Contains(str, pattern)
}

func editDistance(a, b string) int {
	la := len(a)
	lb := len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)

	for j := 0; j <= lb; j++ {
		prev[j] = j
	}

	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = minInt(prev[j]+1, minInt(curr[j-1]+1, prev[j-1]+cost))
		}
		prev, curr = curr, prev
	}

	return prev[lb]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

type BM25Scorer struct {
	idx       *index.Index
	k1        float64
	b         float64
	avgdl     float64
	totalDocs int64
}

func NewBM25Scorer(idx *index.Index) *BM25Scorer {
	return &BM25Scorer{
		idx:       idx,
		k1:        1.2,
		b:         0.75,
		avgdl:     100,
		totalDocs: idx.DocCount(),
	}
}

func (s *BM25Scorer) Score(field, term string, tf int) float64 {
	df := s.idx.MemIndex().DocFreq(term)
	if df == 0 {
		return 0
	}

	idf := math.Log(1 + (float64(s.totalDocs)-float64(df)+0.5)/(float64(df)+0.5))

	numerator := float64(tf) * (s.k1 + 1)
	denominator := float64(tf) + s.k1*(1-s.b+s.b*float64(tf)/s.avgdl)

	return idf * numerator / denominator
}

func (s *Searcher) compareSort(a, b *SearchHit, sortFields []SortField, docs map[string]*index.Document) bool {
	for _, sf := range sortFields {
		if sf.Field == "_score" {
			if a.Score != b.Score {
				if sf.Order == "desc" {
					return a.Score > b.Score
				}
				return a.Score < b.Score
			}
			continue
		}

		docA, okA := docs[a.ID]
		docB, okB := docs[b.ID]
		if !okA || !okB {
			continue
		}

		valA, _ := docA.Fields[sf.Field]
		valB, _ := docB.Fields[sf.Field]

		cmp := compareValues(valA, valB)
		if cmp != 0 {
			if sf.Order == "desc" {
				return cmp > 0
			}
			return cmp < 0
		}
	}
	return a.Score > b.Score
}

func compareValues(a, b interface{}) int {
	fa := toFloat(a)
	fb := toFloat(b)
	if fa < fb {
		return -1
	}
	if fa > fb {
		return 1
	}
	sa := fmt.Sprintf("%v", a)
	sb := fmt.Sprintf("%v", b)
	if sa < sb {
		return -1
	}
	if sa > sb {
		return 1
	}
	return 0
}

func (s *Searcher) getSortValues(hit *SearchHit, sortFields []SortField, docs map[string]*index.Document) []interface{} {
	if len(sortFields) == 0 {
		return nil
	}
	values := make([]interface{}, len(sortFields))
	doc := docs[hit.ID]
	for i, sf := range sortFields {
		if sf.Field == "_score" {
			values[i] = hit.Score
		} else if doc != nil {
			values[i] = doc.Fields[sf.Field]
		}
	}
	return values
}

func (s *Searcher) applySearchAfter(hits []*SearchHit, searchAfter []interface{}, sortFields []SortField, docs map[string]*index.Document) []*SearchHit {
	if len(searchAfter) == 0 || len(sortFields) == 0 {
		return hits
	}

	result := make([]*SearchHit, 0, len(hits))
	for _, hit := range hits {
		values := s.getSortValues(hit, sortFields, docs)
		if s.isAfter(values, searchAfter, sortFields) {
			result = append(result, hit)
		}
	}
	return result
}

func (s *Searcher) isAfter(values []interface{}, after []interface{}, sortFields []SortField) bool {
	for i, sf := range sortFields {
		if i >= len(values) || i >= len(after) {
			return false
		}
		cmp := compareValues(values[i], after[i])
		if sf.Order == "desc" {
			if cmp < 0 {
				return true
			}
			if cmp > 0 {
				return false
			}
		} else {
			if cmp > 0 {
				return true
			}
			if cmp < 0 {
				return false
			}
		}
	}
	return false
}
