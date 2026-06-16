package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

type IndexSettings struct {
	Shards       int `yaml:"shards"`
	FlushSeconds int `yaml:"flush_seconds"`
	MaxMemDocs   int `yaml:"max_mem_docs"`
	MergeFactor  int `yaml:"merge_factor"`
}

type AnalyzerConfig struct {
	Default         string   `yaml:"default"`
	StopWords       []string `yaml:"stop_words"`
	CustomDicts     []string `yaml:"custom_dicts"`
}

type Config struct {
	DataDir     string         `yaml:"data_dir"`
	ServerPort  int            `yaml:"server_port"`
	MetricsPort int            `yaml:"metrics_port"`
	Index       IndexSettings  `yaml:"index"`
	Analyzer    AnalyzerConfig `yaml:"analyzer"`
}

func Default() *Config {
	return &Config{
		DataDir:     "data",
		ServerPort:  8080,
		MetricsPort: 9090,
		Index: IndexSettings{
			Shards:       1,
			FlushSeconds: 5,
			MaxMemDocs:   10000,
			MergeFactor:  4,
		},
		Analyzer: AnalyzerConfig{
			Default: "standard",
			StopWords: []string{
				"的", "了", "是", "在", "我", "有", "和", "就",
				"不", "人", "都", "一", "一个", "上", "也", "很",
				"to", "the", "a", "an", "is", "are", "was", "were",
				"be", "been", "being", "have", "has", "had", "do",
				"does", "did", "will", "would", "could", "should",
				"may", "might", "must", "shall", "can", "need",
				"dare", "ought", "used",
			},
			CustomDicts: []string{},
		},
	}
}

func Load(path string) (*Config, error) {
	cfg := Default()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("read config file: %w", err)
			}
		} else {
			if err := yaml.Unmarshal(data, cfg); err != nil {
				return nil, fmt.Errorf("parse config file: %w", err)
			}
		}
	}

	applyEnvOverrides(cfg)

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.ServerPort = p
		}
	}
	if v := os.Getenv("METRICS"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.MetricsPort = p
		}
	}
}

func validate(cfg *Config) error {
	if cfg.ServerPort <= 0 || cfg.ServerPort > 65535 {
		return fmt.Errorf("invalid server_port: %d", cfg.ServerPort)
	}
	if cfg.MetricsPort <= 0 || cfg.MetricsPort > 65535 {
		return fmt.Errorf("invalid metrics_port: %d", cfg.MetricsPort)
	}
	if cfg.Index.FlushSeconds < 1 {
		return fmt.Errorf("invalid index.flush_seconds: %d", cfg.Index.FlushSeconds)
	}
	if cfg.Index.MaxMemDocs < 100 {
		return fmt.Errorf("invalid index.max_mem_docs: %d", cfg.Index.MaxMemDocs)
	}
	if cfg.Index.MergeFactor < 2 {
		return fmt.Errorf("invalid index.merge_factor: %d", cfg.Index.MergeFactor)
	}
	return nil
}
