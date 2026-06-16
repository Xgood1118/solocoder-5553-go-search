package analyzer

import (
	"strings"
	"unicode"
)

type ChineseTokenizer struct {
	dict map[string]int
	maxLen int
}

func NewChineseTokenizer() *ChineseTokenizer {
	return &ChineseTokenizer{
		dict:   make(map[string]int),
		maxLen: 4,
	}
}

func (t *ChineseTokenizer) LoadDict(words []string) {
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		t.dict[w] = len(w)
		if len(w) > t.maxLen {
			t.maxLen = len(w)
		}
	}
}

func (t *ChineseTokenizer) Tokenize(text string) []Token {
	var tokens []Token
	var current []rune
	position := 0
	offset := 0

	runes := []rune(text)
	i := 0
	for i < len(runes) {
		r := runes[i]
		if isChinese(r) {
			if len(current) > 0 {
				tokens = append(tokens, Token{
					Term:     string(current),
					Position: position,
					Start:    offset - len(string(current)),
					End:      offset,
				})
				position++
				current = current[:0]
			}
			matched := false
			for l := min(t.maxLen, len(runes)-i); l >= 1; l-- {
				word := string(runes[i : i+l])
				if _, ok := t.dict[word]; ok {
					tokens = append(tokens, Token{
						Term:     word,
						Position: position,
						Start:    offset,
						End:      offset + len([]byte(word)),
					})
					position++
					i += l
					offset += len([]byte(word))
					matched = true
					break
				}
			}
			if !matched {
				word := string(r)
				tokens = append(tokens, Token{
					Term:     word,
					Position: position,
					Start:    offset,
					End:      offset + len([]byte(word)),
				})
				position++
				i++
				offset += len([]byte(word))
			}
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, r)
			offset += len([]byte(string(r)))
			i++
		} else {
			if len(current) > 0 {
				tokens = append(tokens, Token{
					Term:     string(current),
					Position: position,
					Start:    offset - len(string(current)),
					End:      offset,
				})
				position++
				current = current[:0]
			}
			offset += len([]byte(string(r)))
			i++
		}
	}

	if len(current) > 0 {
		tokens = append(tokens, Token{
			Term:     string(current),
			Position: position,
			Start:    offset - len(string(current)),
			End:      offset,
		})
	}

	return tokens
}

func isChinese(r rune) bool {
	return unicode.Is(unicode.Scripts["Han"], r)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
