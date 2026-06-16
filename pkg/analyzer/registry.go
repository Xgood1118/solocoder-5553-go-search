package analyzer

import "sync"

type defaultRegistry struct {
	mu        sync.RWMutex
	analyzers map[string]Analyzer
	def       Analyzer
}

func NewRegistry() Registry {
	return &defaultRegistry{
		analyzers: make(map[string]Analyzer),
	}
}

func (r *defaultRegistry) Get(name string) (Analyzer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.analyzers[name]
	return a, ok
}

func (r *defaultRegistry) Register(a Analyzer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.analyzers[a.Name()] = a
	if r.def == nil {
		r.def = a
	}
}

func (r *defaultRegistry) Default() Analyzer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.def
}

func (r *defaultRegistry) SetDefault(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	a, ok := r.analyzers[name]
	if ok {
		r.def = a
	}
	return ok
}
