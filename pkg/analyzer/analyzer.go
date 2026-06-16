package analyzer

type Token struct {
	Term     string
	Position int
	Start    int
	End      int
}

type Analyzer interface {
	Name() string
	Analyze(text string) []Token
}

type Registry interface {
	Get(name string) (Analyzer, bool)
	Register(a Analyzer)
	Default() Analyzer
}
