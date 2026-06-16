package analyzer

import "strings"

type StopWordFilter struct {
	stopWords map[string]struct{}
}

func NewStopWordFilter(words []string) *StopWordFilter {
	m := make(map[string]struct{}, len(words))
	for _, w := range words {
		m[strings.ToLower(w)] = struct{}{}
	}
	return &StopWordFilter{stopWords: m}
}

func (f *StopWordFilter) Filter(tokens []Token) []Token {
	if len(f.stopWords) == 0 {
		return tokens
	}
	result := make([]Token, 0, len(tokens))
	pos := 0
	for _, t := range tokens {
		if _, ok := f.stopWords[strings.ToLower(t.Term)]; !ok {
			t.Position = pos
			result = append(result, t)
			pos++
		}
	}
	return result
}

func (f *StopWordFilter) Add(word string) {
	f.stopWords[strings.ToLower(word)] = struct{}{}
}

func (f *StopWordFilter) Remove(word string) {
	delete(f.stopWords, strings.ToLower(word))
}
