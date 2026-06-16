package query

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/solo/fulltext-search/pkg/index"
)

func (s *Searcher) applyHighlight(hits []*SearchHit, config *HighlightConfig, query Query) {
	if config == nil || len(config.Fields) == 0 {
		return
	}

	terms := s.extractQueryTerms(query)

	preTag := "<em>"
	postTag := "</em>"
	if len(config.PreTags) > 0 {
		preTag = config.PreTags[0]
	}
	if len(config.PostTags) > 0 {
		postTag = config.PostTags[0]
	}

	fragmentSize := config.FragmentSize
	if fragmentSize <= 0 {
		fragmentSize = 60
	}

	for _, hit := range hits {
		highlight := make(map[string][]string)
		for _, field := range config.Fields {
			if value, ok := hit.Source[field]; ok {
				text := fmt.Sprintf("%v", value)
				fragments := s.highlightText(text, terms, fragmentSize, config.NumFragments, preTag, postTag)
				if len(fragments) > 0 {
					highlight[field] = fragments
				}
			}
		}
		hit.Highlight = highlight
	}
}

func (s *Searcher) extractQueryTerms(query Query) []string {
	termsMap := make(map[string]bool)

	var collect func(q Query)
	collect = func(q Query) {
		switch qt := q.(type) {
		case *TermQuery:
			termsMap[strings.ToLower(qt.Value)] = true
		case *TermsQuery:
			for _, v := range qt.Values {
				termsMap[strings.ToLower(v)] = true
			}
		case *MatchQuery:
			tokens := s.idx.GetAnalyzer().Analyze(qt.Query)
			for _, t := range tokens {
				termsMap[strings.ToLower(t.Term)] = true
			}
		case *PrefixQuery:
			inv, ok := s.idx.MemIndex().GetFieldInverted(qt.Field)
			if ok {
				for _, t := range inv.PrefixTerms(qt.Value) {
					termsMap[strings.ToLower(t)] = true
				}
			}
		case *BoolQuery:
			for _, sub := range qt.Must {
				collect(sub)
			}
			for _, sub := range qt.Should {
				collect(sub)
			}
			for _, sub := range qt.Filter {
				collect(sub)
			}
		}
	}

	collect(query)

	terms := make([]string, 0, len(termsMap))
	for t := range termsMap {
		terms = append(terms, t)
	}
	return terms
}

func (s *Searcher) highlightText(text string, terms []string, fragmentSize int, numFragments int, preTag, postTag string) []string {
	if len(terms) == 0 || text == "" {
		return nil
	}

	lowerText := strings.ToLower(text)
	type hitPos struct {
		start int
		end   int
	}
	var hits []hitPos

	runes := []rune(text)
	lowerRunes := []rune(lowerText)

	for _, term := range terms {
		termRunes := []rune(term)
		if len(termRunes) == 0 {
			continue
		}
		for i := 0; i <= len(lowerRunes)-len(termRunes); i++ {
			match := true
			for j := 0; j < len(termRunes); j++ {
				if lowerRunes[i+j] != termRunes[j] {
					match = false
					break
				}
			}
			if match {
				hits = append(hits, hitPos{start: i, end: i + len(termRunes)})
			}
		}
	}

	if len(hits) == 0 {
		start := 0
		end := len(runes)
		if end > fragmentSize {
			end = fragmentSize
		}
		return []string{string(runes[start:end])}
	}

	sort.Slice(hits, func(i, j int) bool {
		return hits[i].start < hits[j].start
	})

	fragmentStart := 0
	fragmentEnd := 0
	bestScore := -1
	bestStart := 0
	bestEnd := 0

	for i, hit := range hits {
		fragmentStart = max(0, hit.start-fragmentSize/2)
		fragmentEnd = min(len(runes), fragmentStart+fragmentSize)
		fragmentStart = max(0, fragmentEnd-fragmentSize)

		score := 0
		for j := i; j < len(hits); j++ {
			if hits[j].start >= fragmentStart && hits[j].end <= fragmentEnd {
				score++
			} else if hits[j].start > fragmentEnd {
				break
			}
		}

		if score > bestScore {
			bestScore = score
			bestStart = fragmentStart
			bestEnd = fragmentEnd
		}
	}

	fragment := runes[bestStart:bestEnd]
	fragmentStr := string(fragment)

	offset := bestStart
	highlighted := fragmentStr
	delta := 0

	for _, hit := range hits {
		if hit.start >= bestStart && hit.end <= bestEnd {
			relStart := hit.start - offset + delta
			relEnd := hit.end - offset + delta

			if relStart < 0 || relEnd > len([]rune(highlighted)) {
				continue
			}

			hlRunes := []rune(highlighted)
			preRunes := []rune(preTag)
			postRunes := []rune(postTag)

			result := make([]rune, 0, len(hlRunes)+len(preRunes)+len(postRunes))
			result = append(result, hlRunes[:relStart]...)
			result = append(result, preRunes...)
			result = append(result, hlRunes[relStart:relEnd]...)
			result = append(result, postRunes...)
			result = append(result, hlRunes[relEnd:]...)

			highlighted = string(result)
			delta += len(preRunes) + len(postRunes)
		}
	}

	return []string{highlighted}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Searcher) computeAggregations(hits []*SearchHit, aggs []Aggregation) map[string]AggregationResult {
	if len(aggs) == 0 {
		return nil
	}

	results := make(map[string]AggregationResult)
	docs := s.getDocsForHits(hits)

	for _, agg := range aggs {
		switch a := agg.(type) {
		case *TermsAggregation:
			results[a.Name()] = s.computeTermsAgg(a, docs)
		case *RangeAggregation:
			results[a.Name()] = s.computeRangeAgg(a, docs)
		case *DateHistogramAggregation:
			results[a.Name()] = s.computeDateHistogramAgg(a, docs)
		}
	}

	return results
}

func (s *Searcher) getDocsForHits(hits []*SearchHit) []*index.Document {
	docs := make([]*index.Document, 0, len(hits))
	for _, hit := range hits {
		if doc, ok := s.idx.GetDocument(hit.ID); ok {
			docs = append(dirs, doc)
		}
	}
	return docs
}

func (s *Searcher) computeTermsAgg(agg *TermsAggregation, docs []*index.Document) AggregationResult {
	counts := make(map[string]int64)

	for _, doc := range docs {
		if value, ok := doc.Fields[agg.Field]; ok {
			key := fmt.Sprintf("%v", value)
			counts[key]++
		}
	}

	type kv struct {
		Key   string
		Count int64
	}
	var sorted []kv
	for k, v := range counts {
		if v >= int64(agg.MinDocCount) {
			sorted = append(sorted, kv{Key: k, Count: v})
		}
	}

	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Count != sorted[j].Count {
			return sorted[i].Count > sorted[j].Count
		}
		return sorted[i].Key < sorted[j].Key
	})

	size := agg.Size
	if size <= 0 || size > len(sorted) {
		size = len(sorted)
	}

	buckets := make([]AggBucket, 0, size)
	for i := 0; i < size; i++ {
		buckets = append(buckets, AggBucket{
			Key:         sorted[i].Key,
			KeyAsString: sorted[i].Key,
			DocCount:    sorted[i].Count,
		})
	}

	return AggregationResult{
		Name:    agg.AggName,
		Type:    "terms",
		Buckets: buckets,
	}
}

func (s *Searcher) computeRangeAgg(agg *RangeAggregation, docs []*index.Document) AggregationResult {
	buckets := make([]AggBucket, len(agg.Ranges))
	for i, r := range agg.Ranges {
		buckets[i] = AggBucket{
			Key:         r.Key,
			KeyAsString: r.Key,
			DocCount:    0,
		}
		if buckets[i].Key == "" {
			buckets[i].Key = fmt.Sprintf("%v-%v", r.From, r.To)
			buckets[i].KeyAsString = buckets[i].Key.(string)
		}
	}

	for _, doc := range docs {
		if value, ok := doc.Fields[agg.Field]; ok {
			v := toFloat(value)
			for i, r := range agg.Ranges {
				from := -1e18
				to := 1e18
				if r.From != nil {
					from = toFloat(r.From)
				}
				if r.To != nil {
					to = toFloat(r.To)
				}
				if v >= from && v < to {
					buckets[i].DocCount++
					break
				}
			}
		}
	}

	return AggregationResult{
		Name:    agg.AggName,
		Type:    "range",
		Buckets: buckets,
	}
}

func (s *Searcher) computeDateHistogramAgg(agg *DateHistogramAggregation, docs []*index.Document) AggregationResult {
	bucketMap := make(map[string]int64)

	for _, doc := range docs {
		if value, ok := doc.Fields[agg.Field]; ok {
			t := parseTime(value)
			if t.IsZero() {
				continue
			}
			bucketKey := truncateTime(t, agg.Interval)
			bucketMap[bucketKey]++
		}
	}

	type kv struct {
		Key   string
		Count int64
	}
	var sorted []kv
	for k, v := range bucketMap {
		if v >= int64(agg.MinDocCount) {
			sorted = append(sorted, kv{Key: k, Count: v})
		}
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Key < sorted[j].Key
	})

	buckets := make([]AggBucket, 0, len(sorted))
	for _, item := range sorted {
		buckets = append(buckets, AggBucket{
			Key:         item.Key,
			KeyAsString: item.Key,
			DocCount:    item.Count,
		})
	}

	return AggregationResult{
		Name:    agg.AggName,
		Type:    "date_histogram",
		Buckets: buckets,
	}
}

func parseTime(v interface{}) time.Time {
	switch val := v.(type) {
	case time.Time:
		return val
	case string:
		for _, layout := range []string{
			time.RFC3339,
			time.RFC3339Nano,
			"2006-01-02T15:04:05",
			"2006-01-02 15:04:05",
			"2006-01-02",
		} {
			if t, err := time.Parse(layout, val); err == nil {
				return t
			}
		}
	case int64:
		return time.Unix(val, 0)
	case float64:
		return time.Unix(int64(val), 0)
	}
	return time.Time{}
}

func truncateTime(t time.Time, interval string) string {
	switch interval {
	case "year":
		return t.Format("2006-01-01T00:00:00Z")
	case "month":
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).Format(time.RFC3339)
	case "week":
		weekday := int(t.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := t.AddDate(0, 0, -weekday+1)
		return time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, t.Location()).Format(time.RFC3339)
	case "day":
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Format(time.RFC3339)
	case "hour":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), 0, 0, 0, t.Location()).Format(time.RFC3339)
	case "minute":
		return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), 0, 0, t.Location()).Format(time.RFC3339)
	default:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Format(time.RFC3339)
	}
}

type SuggestRequest struct {
	Field     string
	Prefix    string
	Size      int
	Fuzzy     bool
	Fuzziness int
}

type SuggestResult struct {
	Text    string
	Score   float64
}

func (s *Searcher) Suggest(req *SuggestRequest) []SuggestResult {
	inv, ok := s.idx.MemIndex().GetFieldInverted(req.Field)
	if !ok {
		return nil
	}

	size := req.Size
	if size <= 0 {
		size = 10
	}

	var results []SuggestResult

	if req.Fuzzy {
		allTerms := inv.AllTerms()
		for _, term := range allTerms {
			dist := editDistance(term, req.Prefix)
			maxDist := req.Fuzziness
			if maxDist == 0 {
				maxDist = 2
			}
			if dist <= maxDist {
				score := 1.0 - float64(dist)/float64(maxDist+1)
				results = append(results, SuggestResult{
					Text:  term,
					Score: score,
				})
			}
		}
	} else {
		terms := inv.PrefixTerms(req.Prefix)
		for _, term := range terms {
			pl, _ := inv.Get(term)
			score := float64(pl.DocFreq)
			results = append(results, SuggestResult{
				Text:  term,
				Score: score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > size {
		results = results[:size]
	}

	return results
}

func isChineseChar(r rune) bool {
	return unicode.Is(unicode.Scripts["Han"], r)
}
