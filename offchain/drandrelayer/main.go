package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
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
		Health:     newDaemonHealth(time.Duration(cfg.PollIntervalSec) * time.Second),
	}

	ctx := context.Background()
	if once {
		if err := relayer.RunOnce(ctx); err != nil {
			log.Fatalf("sync failed: %v", err)
		}
		return
	}

	log.Printf(
		"drand-relayer started (poll=%ds listen=%s min_submit_interval=%ds retry_backoff=%ds tx_inclusion_timeout=%ds max_submit_retries=%d chain_hash=%s)",
		cfg.PollIntervalSec,
		cfg.ListenAddr,
		cfg.MinSubmitIntervalSec,
		cfg.RetryBackoffSec,
		cfg.TxInclusionTimeoutSec,
		cfg.MaxSubmitRetries,
		cfg.DrandChainHash,
	)
	healthServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           healthRoutes(cfg, relayer.Health),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("drand-relayer health listening on %s", cfg.ListenAddr)
		if err := healthServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("health listen: %v", err)
		}
	}()

	ticker := time.NewTicker(time.Duration(cfg.PollIntervalSec) * time.Second)
	defer ticker.Stop()
	for {
		relayer.Health.recordSyncAttempt()
		err := relayer.RunOnce(ctx)
		relayer.Health.recordSyncResult(err)
		if err != nil {
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
