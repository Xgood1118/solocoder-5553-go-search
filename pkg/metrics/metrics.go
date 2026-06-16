package metrics

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

type Counter struct {
	name   string
	help   string
	value  float64
	labels map[string]string
}

type Gauge struct {
	name   string
	help   string
	value  float64
	labels map[string]string
}

type Histogram struct {
	name    string
	help    string
	buckets []float64
	count   uint64
	sum     float64
	values  map[float64]uint64
}

type Registry struct {
	mu         sync.RWMutex
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
}

var defaultRegistry = NewRegistry()

func NewRegistry() *Registry {
	return &Registry{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
	}
}

func DefaultRegistry() *Registry {
	return defaultRegistry
}

func (r *Registry) NewCounter(name, help string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := &Counter{name: name, help: help}
	r.counters[name] = c
	return c
}

func (r *Registry) NewGauge(name, help string) *Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := &Gauge{name: name, help: help}
	r.gauges[name] = g
	return g
}

func (r *Registry) NewHistogram(name, help string, buckets []float64) *Histogram {
	r.mu.Lock()
	defer r.mu.Unlock()
	h := &Histogram{
		name:    name,
		help:    help,
		buckets: buckets,
		values:  make(map[float64]uint64),
	}
	for _, b := range buckets {
		h.values[b] = 0
	}
	r.histograms[name] = h
	return h
}

func (c *Counter) Inc() {
	c.Add(1)
}

func (c *Counter) Add(delta float64) {
	c.value += delta
}

func (c *Counter) Value() float64 {
	return c.value
}

func (g *Gauge) Set(value float64) {
	g.value = value
}

func (g *Gauge) Inc() {
	g.value++
}

func (g *Gauge) Dec() {
	g.value--
}

func (g *Gauge) Value() float64 {
	return g.value
}

func (h *Histogram) Observe(value float64) {
	h.count++
	h.sum += value
	for _, b := range h.buckets {
		if value <= b {
			h.values[b]++
		}
	}
}

type MetricsServer struct {
	port     int
	registry *Registry
}

func NewMetricsServer(port int, reg *Registry) *MetricsServer {
	if reg == nil {
		reg = defaultRegistry
	}
	return &MetricsServer{
		port:     port,
		registry: reg,
	}
}

func (m *MetricsServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", m.handleMetrics)
	addr := fmt.Sprintf(":%d", m.port)
	return http.ListenAndServe(addr, mux)
}

func (m *MetricsServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	m.registry.mu.RLock()
	defer m.registry.mu.RUnlock()

	for _, c := range m.registry.counters {
		fmt.Fprintf(w, "# HELP %s %s\n", c.name, c.help)
		fmt.Fprintf(w, "# TYPE %s counter\n", c.name)
		fmt.Fprintf(w, "%s %f\n", c.name, c.value)
	}

	for _, g := range m.registry.gauges {
		fmt.Fprintf(w, "# HELP %s %s\n", g.name, g.help)
		fmt.Fprintf(w, "# TYPE %s gauge\n", g.name)
		fmt.Fprintf(w, "%s %f\n", g.name, g.value)
	}

	for _, h := range m.registry.histograms {
		fmt.Fprintf(w, "# HELP %s %s\n", h.name, h.help)
		fmt.Fprintf(w, "# TYPE %s histogram\n", h.name)

		var cumulative uint64
		for _, b := range h.buckets {
			cumulative += h.values[b]
			fmt.Fprintf(w, "%s_bucket{le=\"%g\"} %d\n", h.name, b, cumulative)
		}
		fmt.Fprintf(w, "%s_bucket{le=\"+Inf\"} %d\n", h.name, h.count)
		fmt.Fprintf(w, "%s_sum %f\n", h.name, h.sum)
		fmt.Fprintf(w, "%s_count %d\n", h.name, h.count)
	}
}

type SearchMetrics struct {
	RequestsTotal   *Counter
	RequestDuration *Histogram
	IndexDocsTotal  *Gauge
	IndexSizeBytes  *Gauge
	UpsertsTotal    *Counter
	DeletesTotal    *Counter
}

var searchMetrics *SearchMetrics

func InitSearchMetrics(reg *Registry) *SearchMetrics {
	if reg == nil {
		reg = defaultRegistry
	}
	searchMetrics = &SearchMetrics{
		RequestsTotal:   reg.NewCounter("search_requests_total", "Total number of search requests"),
		RequestDuration: reg.NewHistogram("search_request_duration_seconds", "Search request duration in seconds", []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1.0, 5.0}),
		IndexDocsTotal:  reg.NewGauge("index_docs_total", "Total number of documents in index"),
		IndexSizeBytes:  reg.NewGauge("index_size_bytes", "Total size of index in bytes"),
		UpsertsTotal:    reg.NewCounter("upserts_total", "Total number of document upserts"),
		DeletesTotal:    reg.NewCounter("deletes_total", "Total number of document deletes"),
	}
	return searchMetrics
}

func GetSearchMetrics() *SearchMetrics {
	return searchMetrics
}

func (m *SearchMetrics) RecordSearch(duration time.Duration) {
	m.RequestsTotal.Inc()
	m.RequestDuration.Observe(duration.Seconds())
}

func (m *SearchMetrics) RecordUpsert() {
	m.UpsertsTotal.Inc()
}

func (m *SearchMetrics) RecordDelete() {
	m.DeletesTotal.Inc()
}

func (m *SearchMetrics) SetDocCount(n int64) {
	m.IndexDocsTotal.Set(float64(n))
}

func (m *SearchMetrics) SetSizeBytes(n int64) {
	m.IndexSizeBytes.Set(float64(n))
}
