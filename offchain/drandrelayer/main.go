package main

import (
	"context"
	"flag"
	"log"
	"os"
	"path/filepath"
	"time"
)

func main() {
	var cfgPath string
	var once bool
	flag.StringVar(&cfgPath, "config", defaultConfigPath(), "path to drandrelayer config json")
	flag.BoolVar(&once, "once", false, "run one sync iteration")
	flag.Parse()

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	chain, err := NewChainClient(cfg.GRPCAddr)
	if err != nil {
		log.Fatalf("failed to connect to chain gRPC: %v", err)
	}
	defer chain.Close()

	relayer := &Relayer{
		Cfg:        cfg,
		Chain:      chain,
		HTTPClient: (httpClientWithTimeout{TimeoutSec: cfg.RequestTimeoutSec}).Client(),
	}

	ctx := context.Background()
	if once {
		if err := relayer.RunOnce(ctx); err != nil {
			log.Fatalf("sync failed: %v", err)
		}
		return
	}

	log.Printf("drand-relayer started (poll=%ds chain_hash=%s)", cfg.PollIntervalSec, cfg.DrandChainHash)
	ticker := time.NewTicker(time.Duration(cfg.PollIntervalSec) * time.Second)
	defer ticker.Stop()
	for {
		if err := relayer.RunOnce(ctx); err != nil {
			log.Printf("sync error: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func defaultConfigPath() string {
	wd, err := os.Getwd()
	if err != nil {
		return "offchain/drandrelayer/config.json"
	}
	return filepath.Join(wd, "offchain", "drandrelayer", "config.json")
}
