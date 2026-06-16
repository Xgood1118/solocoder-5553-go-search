package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/solo/fulltext-search/pkg/analyzer"
	"github.com/solo/fulltext-search/pkg/config"
	"github.com/solo/fulltext-search/pkg/index"
	"github.com/solo/fulltext-search/pkg/metrics"
	"github.com/solo/fulltext-search/pkg/server"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	reg := analyzer.NewRegistry()

	standard := analyzer.NewStandardAnalyzer(cfg.Analyzer.StopWords, cfg.Analyzer.CustomDicts)
	reg.Register(standard)
	reg.SetDefault(cfg.Analyzer.Default)

	simple := analyzer.NewSimpleAnalyzer()
	reg.Register(simple)

	im := index.NewIndexManager(cfg, reg)

	metricsReg := metrics.NewRegistry()
	metrics.InitSearchMetrics(metricsReg)

	srv := server.NewServer(im, cfg.ServerPort)
	metricsSrv := metrics.NewMetricsServer(cfg.MetricsPort, metricsReg)

	go func() {
		log.Printf("Starting metrics server on :%d", cfg.MetricsPort)
		if err := metricsSrv.Start(); err != nil {
			log.Printf("Metrics server error: %v", err)
		}
	}()

	go func() {
		log.Printf("Starting search server on :%d", cfg.ServerPort)
		log.Printf("API base path: /v1/indexes")
		log.Printf("Health check: /healthz, /readyz")
		if err := srv.Start(); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nShutting down server...")
	im.Close()
	fmt.Println("Server stopped")
}
