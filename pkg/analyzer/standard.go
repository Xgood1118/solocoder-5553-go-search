package analyzer

import (
	"strings"
)

type StandardAnalyzer struct {
	chineseTokenizer *ChineseTokenizer
	stemmer          *PorterStemmer
	stopFilter       *StopWordFilter
}

func NewStandardAnalyzer(stopWords []string, customDicts []string) *StandardAnalyzer {
	ct := NewChineseTokenizer()
	for _, dict := range customDicts {
		ct.dict[dict] = len(dict)
		if len(dict) > ct.maxLen {
			ct.maxLen = len(dict)
		}
	}
	return &StandardAnalyzer{
		chineseTokenizer: ct,
		stemmer:          NewPorterStemmer(),
		stopFilter:       NewStopWordFilter(stopWords),
	}
}

func (a *StandardAnalyzer) Name() string {
	return "standard"
}

func (a *StandardAnalyzer) Analyze(text string) []Token {
	tokens := a.chineseTokenizer.Tokenize(text)
	tokens = lowercaseFilter(tokens)
	tokens = a.stemmerFilter(tokens)
	tokens = a.stopFilter.Filter(tokens)
	return tokens
}

func (a *StandardAnalyzer) stemmerFilter(tokens []Token) []Token {
	for i := range tokens {
		if isEnglishWord(tokens[i].Term) {
			tokens[i].Term = a.stemmer.Stem(tokens[i].Term)
		}
	}
	return tokens
}

func lowercaseFilter(tokens []Token) []Token {
	for i := range tokens {
		tokens[i].Term = strings.ToLower(tokens[i].Term)
	}
	return tokens
}

func isEnglishWord(s string) bool {
	for _, r := range s {
		if r < 'a' || r > 'z' {
			if r < 'A' || r > 'Z' {
				return false
			}
		}
	}
	return len(s) > 0
}

type SimpleAnalyzer struct{}

func NewSimpleAnalyzer() *SimpleAnalyzer {
	return &SimpleAnalyzer{}
}

func (a *SimpleAnalyzer) Name() string {
	return "simple"
}

func (a *SimpleAnalyzer) Analyze(text string) []Token {
	var tokens []Token
	var current []rune
	position := 0
	offset := 0

	for _, r := range text {
		if isTokenChar(r) {
			current = append(current, r)
			offset += len([]byte(string(r)))
		} else {
			if len(current) > 0 {
				tokens = append(tokens, Token{
					Term:     strings.ToLower(string(current)),
					Position: position,
					Start:    offset - len(string(current)),
					End:      offset,
				})
				position++
				current = current[:0]
			}
			offset += len([]byte(string(r)))
		}
	}

	if len(current) > 0 {
		tokens = append(tokens, Token{
			Term:     strings.ToLower(string(current)),
			Position: position,
			Start:    offset - len(string(current)),
			End:      offset,
		})
	}

	return tokens
}

func isTokenChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		(r >= 0x4e00 && r <= 0x9fff)
}
