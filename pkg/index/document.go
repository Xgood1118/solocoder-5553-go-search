package index

import "time"

type Document struct {
	ID        string                 `json:"_id"`
	Fields    map[string]interface{} `json:"_source"`
	Score     float64                `json:"_score,omitempty"`
	Tombstone bool                   `json:"-"`
	Version   int64                  `json:"_version,omitempty"`
	CreatedAt time.Time              `json:"-"`
	UpdatedAt time.Time              `json:"-"`
}

type FieldMapping struct {
	Type       string `json:"type"`
	Analyzer   string `json:"analyzer,omitempty"`
	Store      bool   `json:"store,omitempty"`
	Indexed    bool   `json:"index,omitempty"`
	DocValues  bool   `json:"doc_values,omitempty"`
}

type IndexMapping struct {
	Properties map[string]FieldMapping `json:"properties"`
}

type IndexSettings struct {
	NumberOfShards   int    `json:"number_of_shards,omitempty"`
	NumberOfReplicas int    `json:"number_of_replicas,omitempty"`
	Analyzer         string `json:"analyzer,omitempty"`
}

type IndexMeta struct {
	Name     string        `json:"name"`
	Mapping  IndexMapping  `json:"mapping"`
	Settings IndexSettings `json:"settings"`
	Created  time.Time     `json:"created"`
	Updated  time.Time     `json:"updated"`
	DocCount int64         `json:"doc_count"`
}
