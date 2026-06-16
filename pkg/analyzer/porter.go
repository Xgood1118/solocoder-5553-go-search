package analyzer

import "strings"

type PorterStemmer struct{}

func NewPorterStemmer() *PorterStemmer {
	return &PorterStemmer{}
}

func (p *PorterStemmer) Stem(word string) string {
	if len(word) < 3 {
		return strings.ToLower(word)
	}
	w := strings.ToLower(word)
	w = step1a(w)
	w = step1b(w)
	w = step1c(w)
	w = step2(w)
	w = step3(w)
	w = step4(w)
	w = step5a(w)
	w = step5b(w)
	return w
}

func isConsonant(word string, i int) bool {
	switch word[i] {
	case 'a', 'e', 'i', 'o', 'u':
		return false
	case 'y':
		if i == 0 {
			return true
		}
		return !isConsonant(word, i-1)
	default:
		return true
	}
}

func measure(word string) int {
	count := 0
	prevVowel := false
	for i := 0; i < len(word); i++ {
		if isConsonant(word, i) {
			if prevVowel {
				count++
				prevVowel = false
			}
		} else {
			prevVowel = true
		}
	}
	return count
}

func containsVowel(word string) bool {
	for i := 0; i < len(word); i++ {
		if !isConsonant(word, i) {
			return true
		}
	}
	return false
}

func endsWithDoubleConsonant(word string) bool {
	if len(word) < 2 {
		return false
	}
	if word[len(word)-1] != word[len(word)-2] {
		return false
	}
	return isConsonant(word, len(word)-1)
}

func cvc(word string) bool {
	if len(word) < 3 {
		return false
	}
	if !isConsonant(word, len(word)-1) {
		return false
	}
	if word[len(word)-1] == 'w' || word[len(word)-1] == 'x' || word[len(word)-1] == 'y' {
		return false
	}
	if isConsonant(word, len(word)-2) {
		return false
	}
	if !isConsonant(word, len(word)-3) {
		return false
	}
	return true
}

func step1a(word string) string {
	if strings.HasSuffix(word, "sses") {
		return word[:len(word)-2]
	}
	if strings.HasSuffix(word, "ies") {
		return word[:len(word)-2]
	}
	if strings.HasSuffix(word, "ss") {
		return word
	}
	if strings.HasSuffix(word, "s") {
		return word[:len(word)-1]
	}
	return word
}

func step1b(word string) string {
	if strings.HasSuffix(word, "eed") {
		stem := word[:len(word)-3]
		if measure(stem) > 0 {
			return stem + "ee"
		}
		return word
	}
	if strings.HasSuffix(word, "ed") {
		stem := word[:len(word)-2]
		if containsVowel(stem) {
			return step1b2(stem)
		}
		return word
	}
	if strings.HasSuffix(word, "ing") {
		stem := word[:len(word)-3]
		if containsVowel(stem) {
			return step1b2(stem)
		}
		return word
	}
	return word
}

func step1b2(word string) string {
	if strings.HasSuffix(word, "at") {
		return word + "e"
	}
	if strings.HasSuffix(word, "bl") {
		return word + "e"
	}
	if strings.HasSuffix(word, "iz") {
		return word + "e"
	}
	if endsWithDoubleConsonant(word) {
		if word[len(word)-1] != 'l' && word[len(word)-1] != 's' && word[len(word)-1] != 'z' {
			return word[:len(word)-1]
		}
	}
	if measure(word) == 1 && cvc(word) {
		return word + "e"
	}
	return word
}

func step1c(word string) string {
	if strings.HasSuffix(word, "y") {
		stem := word[:len(word)-1]
		if containsVowel(stem) {
			return stem + "i"
		}
	}
	return word
}

var step2Suffixes = []struct {
	suffix   string
	replaced string
}{
	{"ational", "ate"},
	{"tional", "tion"},
	{"enci", "ence"},
	{"anci", "ance"},
	{"izer", "ize"},
	{"abli", "able"},
	{"alli", "al"},
	{"entli", "ent"},
	{"eli", "e"},
	{"ousli", "ous"},
	{"ization", "ize"},
	{"ation", "ate"},
	{"ator", "ate"},
	{"alism", "al"},
	{"iveness", "ive"},
	{"fulness", "ful"},
	{"ousness", "ous"},
	{"aliti", "al"},
	{"iviti", "ive"},
	{"biliti", "ble"},
}

func step2(word string) string {
	for _, s := range step2Suffixes {
		if strings.HasSuffix(word, s.suffix) {
			stem := word[:len(word)-len(s.suffix)]
			if measure(stem) > 0 {
				return stem + s.replaced
			}
			return word
		}
	}
	return word
}

var step3Suffixes = []struct {
	suffix   string
	replaced string
}{
	{"icate", "ic"},
	{"ative", ""},
	{"alize", "al"},
	{"iciti", "ic"},
	{"ical", "ic"},
	{"ful", ""},
	{"ness", ""},
}

func step3(word string) string {
	for _, s := range step3Suffixes {
		if strings.HasSuffix(word, s.suffix) {
			stem := word[:len(word)-len(s.suffix)]
			if measure(stem) > 0 {
				return stem + s.replaced
			}
			return word
		}
	}
	return word
}

var step4Suffixes = []string{
	"al", "ance", "ence", "er", "ic", "able", "ible",
	"ant", "ement", "ment", "ent", "sion", "tion",
	"ou", "ism", "ate", "iti", "ous", "ive", "ize",
}

func step4(word string) string {
	for _, s := range step4Suffixes {
		if strings.HasSuffix(word, s) {
			stem := word[:len(word)-len(s)]
			if measure(stem) > 1 {
				return stem
			}
			return word
		}
	}
	return word
}

func step5a(word string) string {
	if strings.HasSuffix(word, "e") {
		stem := word[:len(word)-1]
		if measure(stem) > 1 {
			return stem
		}
		if measure(stem) == 1 && !cvc(stem) {
			return stem
		}
	}
	return word
}

func step5b(word string) string {
	if strings.HasSuffix(word, "l") && measure(word) > 1 && endsWithDoubleConsonant(word) {
		return word[:len(word)-1]
	}
	return word
}

func (p *PorterStemmer) Filter(tokens []Token) []Token {
	for i := range tokens {
		tokens[i].Term = p.Stem(tokens[i].Term)
	}
	return tokens
}
