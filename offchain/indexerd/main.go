package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"content-grid-chain/offchain/executor"
)

func main() {
	var cfgPath string
	var once bool
	flag.StringVar(&cfgPath, "config", defaultConfigPath(), "path to indexerd config json")
	flag.BoolVar(&once, "once", false, "index once on startup and exit")
	flag.Parse()

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	fetcher := executor.NewHTTPFetcher(cfg.FetchTimeout())

	var chain *ChainClient
	if cfg.ChainGRPCAddr != "" {
		c, err := NewChainClient(cfg.ChainGRPCAddr)
		if err != nil {
			log.Fatalf("failed to init chain client: %v", err)
		}
		chain = c
		defer func() {
			_ = chain.Close()
		}()
	}

	store := NewStore()
	chroma := NewChromaClient(cfg.ChromaBaseURL, cfg.ChromaCollection)
	log.Printf("chroma enabled: base_url=%s collection=%s", cfg.ChromaBaseURL, cfg.ChromaCollection)
	indexer := &Indexer{Cfg: cfg, Store: store, Fetcher: fetcher, Chain: chain, Chroma: chroma}

	ctx := context.Background()
	if once {
		indexer.IndexAll(ctx)
		return
	}

	// background indexing loop
	go func() {
		ticker := time.NewTicker(cfg.IndexInterval())
		defer ticker.Stop()
		indexer.IndexAll(ctx)
		for range ticker.C {
			indexer.IndexAll(ctx)
		}
	}()

	srv := &Server{Cfg: cfg, Store: store, Indexer: indexer, Chroma: chroma}
	mode := "static"
	if cfg.ChainGRPCAddr != "" {
		mode = "chain"
	}
	log.Printf("indexerd listening on %s (publisher_source=%s interval=%s signature_bits=%d)", cfg.ListenAddr, mode, cfg.IndexInterval(), cfg.SignatureBits)
	log.Fatal(http.ListenAndServe(cfg.ListenAddr, srv.routes()))
}

func defaultConfigPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return "offchain/indexerd/config.json"
	}
	return filepath.Join(wd, "offchain", "indexerd", "config.json")
}
