package query

import (
	"encoding/json"
	"fmt"
)

type Query interface {
	Type() string
}

type TermQuery struct {
	Field string
	Value string
}

func (q *TermQuery) Type() string { return "term" }

type TermsQuery struct {
	Field  string
	Values []string
}

func (q *TermsQuery) Type() string { return "terms" }

type MatchQuery struct {
	Field string
	Query string
}

func (q *MatchQuery) Type() string { return "match" }

type MatchAllQuery struct{}

func (q *MatchAllQuery) Type() string { return "match_all" }

type RangeQuery struct {
	Field string
	Gte   interface{}
	Gt    interface{}
	Lte   interface{}
	Lt    interface{}
}

func (q *RangeQuery) Type() string { return "range" }

type ExistsQuery struct {
	Field string
}

func (q *ExistsQuery) Type() string { return "exists" }

type PrefixQuery struct {
	Field string
	Value string
}

func (q *PrefixQuery) Type() string { return "prefix" }

type WildcardQuery struct {
	Field string
	Value string
}

func (q *WildcardQuery) Type() string { return "wildcard" }

type RegexQuery struct {
	Field string
	Value string
}

func (q *RegexQuery) Type() string { return "regex" }

type FuzzyQuery struct {
	Field      string
	Value      string
	Fuzziness  int
	PrefixLen  int
}

func (q *FuzzyQuery) Type() string { return "fuzzy" }

type BoolQuery struct {
	Must    []Query
	Should  []Query
	MustNot []Query
	Filter  []Query
}

func (q *BoolQuery) Type() string { return "bool" }

type SortField struct {
	Field     string
	Order     string
	Missing   string
	Mode      string
}

type HighlightConfig struct {
	Fields       []string
	PreTags      []string
	PostTags     []string
	FragmentSize int
	NumFragments int
}

type Aggregation interface {
	Type() string
	Name() string
}

type TermsAggregation struct {
	AggName   string
	Field     string
	Size      int
	MinDocCount int
}

func (a *TermsAggregation) Type() string { return "terms" }
func (a *TermsAggregation) Name() string { return a.AggName }

type RangeAggregation struct {
	AggName string
	Field   string
	Ranges  []RangeBucket
}

type RangeBucket struct {
	Key string
	From interface{}
	To   interface{}
}

func (a *RangeAggregation) Type() string { return "range" }
func (a *RangeAggregation) Name() string { return a.AggName }

type DateHistogramAggregation struct {
	AggName      string
	Field        string
	Interval     string
	Format       string
	MinDocCount  int
}

func (a *DateHistogramAggregation) Type() string { return "date_histogram" }
func (a *DateHistogramAggregation) Name() string { return a.AggName }

type SearchRequest struct {
	Query        Query
	From         int
	Size         int
	Sort         []SortField
	SearchAfter  []interface{}
	Highlight    *HighlightConfig
	Aggregations []Aggregation
	Source       []string
}

func ParseQueryDSL(raw map[string]interface{}) (Query, error) {
	if len(raw) == 0 {
		return &MatchAllQuery{}, nil
	}

	for key, value := range raw {
		switch key {
		case "term":
			return parseTermQuery(value)
		case "terms":
			return parseTermsQuery(value)
		case "match":
			return parseMatchQuery(value)
		case "match_all":
			return &MatchAllQuery{}, nil
		case "range":
			return parseRangeQuery(value)
		case "exists":
			return parseExistsQuery(value)
		case "prefix":
			return parsePrefixQuery(value)
		case "wildcard":
			return parseWildcardQuery(value)
		case "regex":
			return parseRegexQuery(value)
		case "fuzzy":
			return parseFuzzyQuery(value)
		case "bool":
			return parseBoolQuery(value)
		default:
			return nil, fmt.Errorf("unknown query type: %s", key)
		}
	}

	return &MatchAllQuery{}, nil
}

func parseTermQuery(v interface{}) (Query, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid term query")
	}
	for field, value := range m {
		switch val := value.(type) {
		case map[string]interface{}:
			if vv, ok := val["value"]; ok {
				return &TermQuery{Field: field, Value: fmt.Sprintf("%v", vv)}, nil
			}
			return nil, fmt.Errorf("term query missing value")
		default:
			return &TermQuery{Field: field, Value: fmt.Sprintf("%v", val)}, nil
		}
	}
	return nil, fmt.Errorf("empty term query")
}

func parseTermsQuery(v interface{}) (Query, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid terms query")
	}
	for field, value := range m {
		arr, ok := value.([]interface{})
		if !ok {
			return nil, fmt.Errorf("terms query value must be array")
		}
		vals := make([]string, len(arr))
		for i, a := range arr {
			vals[i] = fmt.Sprintf("%v", a)
		}
		return &TermsQuery{Field: field, Values: vals}, nil
	}
	return nil, fmt.Errorf("empty terms query")
}

func parseMatchQuery(v interface{}) (Query, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid match query")
	}
	for field, value := range m {
		switch val := value.(type) {
		case map[string]interface{}:
			if q, ok := val["query"]; ok {
				return &MatchQuery{Field: field, Query: fmt.Sprintf("%v", q)}, nil
			}
			return nil, fmt.Errorf("match query missing query")
		default:
			return &MatchQuery{Field: field, Query: fmt.Sprintf("%v", val)}, nil
		}
	}
	return nil, fmt.Errorf("empty match query")
}

func parseRangeQuery(v interface{}) (Query, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid range query")
	}
	for field, value := range m {
		params, ok := value.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("range query params must be object")
		}
		q := &RangeQuery{Field: field}
		if v, ok := params["gte"]; ok {
			q.Gte = v
		}
		if v, ok := params["gt"]; ok {
			q.Gt = v
		}
		if v, ok := params["lte"]; ok {
			q.Lte = v
		}
		if v, ok := params["lt"]; ok {
			q.Lt = v
		}
		return q, nil
	}
	return nil, fmt.Errorf("empty range query")
}

func parseExistsQuery(v interface{}) (Query, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid exists query")
	}
	field, ok := m["field"]
	if !ok {
		return nil, fmt.Errorf("exists query missing field")
	}
	return &ExistsQuery{Field: fmt.Sprintf("%v", field)}, nil
}

func parsePrefixQuery(v interface{}) (Query, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid prefix query")
	}
	for field, value := range m {
		switch val := value.(type) {
		case map[string]interface{}:
			if v, ok := val["value"]; ok {
				return &PrefixQuery{Field: field, Value: fmt.Sprintf("%v", v)}, nil
			}
			return nil, fmt.Errorf("prefix query missing value")
		default:
			return &PrefixQuery{Field: field, Value: fmt.Sprintf("%v", val)}, nil
		}
	}
	return nil, fmt.Errorf("empty prefix query")
}

func parseWildcardQuery(v interface{}) (Query, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid wildcard query")
	}
	for field, value := range m {
		switch val := value.(type) {
		case map[string]interface{}:
			if v, ok := val["value"]; ok {
				return &WildcardQuery{Field: field, Value: fmt.Sprintf("%v", v)}, nil
			}
			return nil, fmt.Errorf("wildcard query missing value")
		default:
			return &WildcardQuery{Field: field, Value: fmt.Sprintf("%v", val)}, nil
		}
	}
	return nil, fmt.Errorf("empty wildcard query")
}

func parseRegexQuery(v interface{}) (Query, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid regex query")
	}
	for field, value := range m {
		switch val := value.(type) {
		case map[string]interface{}:
			if v, ok := val["value"]; ok {
				return &RegexQuery{Field: field, Value: fmt.Sprintf("%v", v)}, nil
			}
			return nil, fmt.Errorf("regex query missing value")
		default:
			return &RegexQuery{Field: field, Value: fmt.Sprintf("%v", val)}, nil
		}
	}
	return nil, fmt.Errorf("empty regex query")
}

func parseFuzzyQuery(v interface{}) (Query, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid fuzzy query")
	}
	for field, value := range m {
		q := &FuzzyQuery{Field: field, Fuzziness: 2, PrefixLen: 0}
		switch val := value.(type) {
		case map[string]interface{}:
			if v, ok := val["value"]; ok {
				q.Value = fmt.Sprintf("%v", v)
			}
			if v, ok := val["fuzziness"]; ok {
				q.Fuzziness = int(v.(float64))
			}
			if v, ok := val["prefix_length"]; ok {
				q.PrefixLen = int(v.(float64))
			}
		default:
			q.Value = fmt.Sprintf("%v", val)
		}
		return q, nil
	}
	return nil, fmt.Errorf("empty fuzzy query")
}

func parseBoolQuery(v interface{}) (Query, error) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid bool query")
	}

	q := &BoolQuery{}

	if must, ok := m["must"]; ok {
		queries, err := parseQueryArray(must)
		if err != nil {
			return nil, err
		}
		q.Must = queries
	}

	if should, ok := m["should"]; ok {
		queries, err := parseQueryArray(should)
		if err != nil {
			return nil, err
		}
		q.Should = queries
	}

	if mustNot, ok := m["must_not"]; ok {
		queries, err := parseQueryArray(mustNot)
		if err != nil {
			return nil, err
		}
		q.MustNot = queries
	}

	if filter, ok := m["filter"]; ok {
		queries, err := parseQueryArray(filter)
		if err != nil {
			return nil, err
		}
		q.Filter = queries
	}

	return q, nil
}

func parseQueryArray(v interface{}) ([]Query, error) {
	switch val := v.(type) {
	case []interface{}:
		result := make([]Query, 0, len(val))
		for _, item := range val {
			q, err := ParseQueryDSL(item.(map[string]interface{}))
			if err != nil {
				return nil, err
			}
			result = append(result, q)
		}
		return result, nil
	case map[string]interface{}:
		q, err := ParseQueryDSL(val)
		if err != nil {
			return nil, err
		}
		return []Query{q}, nil
	default:
		return nil, fmt.Errorf("invalid query array")
	}
}

func ParseSort(raw []interface{}) []SortField {
	result := make([]SortField, 0, len(raw))
	for _, item := range raw {
		switch val := item.(type) {
		case string:
			if val == "_score" || val == "_doc" {
				result = append(result, SortField{Field: val, Order: "desc"})
			} else {
				result = append(result, SortField{Field: val, Order: "asc"})
			}
		case map[string]interface{}:
			for field, params := range val {
				sf := SortField{Field: field}
				if p, ok := params.(map[string]interface{}); ok {
					if order, ok := p["order"]; ok {
						sf.Order = fmt.Sprintf("%v", order)
					}
				}
				if sf.Order == "" {
					if field == "_score" {
						sf.Order = "desc"
					} else {
						sf.Order = "asc"
					}
				}
				result = append(result, sf)
			}
		}
	}
	return result
}

func ParseHighlight(raw map[string]interface{}) *HighlightConfig {
	if raw == nil {
		return nil
	}
	hl := &HighlightConfig{
		FragmentSize: 60,
		NumFragments: 1,
		PreTags:      []string{"<em>"},
		PostTags:     []string{"</em>"},
	}

	if fields, ok := raw["fields"]; ok {
		if arr, ok := fields.([]interface{}); ok {
			for _, f := range arr {
				if fm, ok := f.(map[string]interface{}); ok {
					for fieldName := range fm {
						hl.Fields = append(hl.Fields, fieldName)
					}
				} else if s, ok := f.(string); ok {
					hl.Fields = append(hl.Fields, s)
				}
			}
		}
	}

	if preTags, ok := raw["pre_tags"]; ok {
		if arr, ok := preTags.([]interface{}); ok {
			tags := make([]string, len(arr))
			for i, t := range arr {
				tags[i] = fmt.Sprintf("%v", t)
			}
			hl.PreTags = tags
		}
	}

	if postTags, ok := raw["post_tags"]; ok {
		if arr, ok := postTags.([]interface{}); ok {
			tags := make([]string, len(arr))
			for i, t := range arr {
				tags[i] = fmt.Sprintf("%v", t)
			}
			hl.PostTags = tags
		}
	}

	if fragmentSize, ok := raw["fragment_size"]; ok {
		if n, ok := fragmentSize.(float64); ok {
			hl.FragmentSize = int(n)
		}
	}

	if numFragments, ok := raw["number_of_fragments"]; ok {
		if n, ok := numFragments.(float64); ok {
			hl.NumFragments = int(n)
		}
	}

	return hl
}

func ParseAggregations(raw map[string]interface{}) ([]Aggregation, error) {
	if raw == nil {
		return nil, nil
	}

	var result []Aggregation
	for name, value := range raw {
		aggMap, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		for aggType, params := range aggMap {
			switch aggType {
			case "terms":
				agg, err := parseTermsAggregation(name, params)
				if err != nil {
					return nil, err
				}
				result = append(result, agg)
			case "range":
				agg, err := parseRangeAggregation(name, params)
				if err != nil {
					return nil, err
				}
				result = append(result, agg)
			case "date_histogram":
				agg, err := parseDateHistogramAggregation(name, params)
				if err != nil {
					return nil, err
				}
				result = append(result, agg)
			}
		}
	}
	return result, nil
}

func parseTermsAggregation(name string, params interface{}) (Aggregation, error) {
	p, ok := params.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid terms aggregation")
	}
	agg := &TermsAggregation{AggName: name, Size: 10, MinDocCount: 1}
	if field, ok := p["field"]; ok {
		agg.Field = fmt.Sprintf("%v", field)
	}
	if size, ok := p["size"]; ok {
		agg.Size = int(size.(float64))
	}
	if minDoc, ok := p["min_doc_count"]; ok {
		agg.MinDocCount = int(minDoc.(float64))
	}
	return agg, nil
}

func parseRangeAggregation(name string, params interface{}) (Aggregation, error) {
	p, ok := params.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid range aggregation")
	}
	agg := &RangeAggregation{AggName: name}
	if field, ok := p["field"]; ok {
		agg.Field = fmt.Sprintf("%v", field)
	}
	if ranges, ok := p["ranges"]; ok {
		if arr, ok := ranges.([]interface{}); ok {
			for _, r := range arr {
				rm, ok := r.(map[string]interface{})
				if !ok {
					continue
				}
				bucket := RangeBucket{}
				if key, ok := rm["key"]; ok {
					bucket.Key = fmt.Sprintf("%v", key)
				}
				if from, ok := rm["from"]; ok {
					bucket.From = from
				}
				if to, ok := rm["to"]; ok {
					bucket.To = to
				}
				agg.Ranges = append(agg.Ranges, bucket)
			}
		}
	}
	return agg, nil
}

func parseDateHistogramAggregation(name string, params interface{}) (Aggregation, error) {
	p, ok := params.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid date_histogram aggregation")
	}
	agg := &DateHistogramAggregation{AggName: name, Interval: "day", MinDocCount: 0}
	if field, ok := p["field"]; ok {
		agg.Field = fmt.Sprintf("%v", field)
	}
	if interval, ok := p["interval"]; ok {
		agg.Interval = fmt.Sprintf("%v", interval)
	}
	if format, ok := p["format"]; ok {
		agg.Format = fmt.Sprintf("%v", format)
	}
	if minDoc, ok := p["min_doc_count"]; ok {
		agg.MinDocCount = int(minDoc.(float64))
	}
	return agg, nil
}

func ParseSearchRequest(body json.RawMessage) (*SearchRequest, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	req := &SearchRequest{
		Size: 10,
	}

	if q, ok := raw["query"]; ok {
		query, err := ParseQueryDSL(q.(map[string]interface{}))
		if err != nil {
			return nil, err
		}
		req.Query = query
	} else {
		req.Query = &MatchAllQuery{}
	}

	if from, ok := raw["from"]; ok {
		req.From = int(from.(float64))
	}
	if size, ok := raw["size"]; ok {
		req.Size = int(size.(float64))
	}

	if sort, ok := raw["sort"]; ok {
		req.Sort = ParseSort(sort.([]interface{}))
	}

	if searchAfter, ok := raw["search_after"]; ok {
		if arr, ok := searchAfter.([]interface{}); ok {
			req.SearchAfter = arr
		}
	}

	if highlight, ok := raw["highlight"]; ok {
		req.Highlight = ParseHighlight(highlight.(map[string]interface{}))
	}

	if aggs, ok := raw["aggs"]; ok {
		aggregations, err := ParseAggregations(aggs.(map[string]interface{}))
		if err != nil {
			return nil, err
		}
		req.Aggregations = aggregations
	}
	if aggs, ok := raw["aggregations"]; ok {
		aggregations, err := ParseAggregations(aggs.(map[string]interface{}))
		if err != nil {
			return nil, err
		}
		req.Aggregations = aggregations
	}

	if source, ok := raw["_source"]; ok {
		switch s := source.(type) {
		case []interface{}:
			fields := make([]string, len(s))
			for i, f := range s {
				fields[i] = fmt.Sprintf("%v", f)
			}
			req.Source = fields
		case bool:
			if !s {
				req.Source = []string{}
			}
		}
	}

	return req, nil
}
