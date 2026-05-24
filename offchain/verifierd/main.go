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

	registryoffchain "content-grid-chain/offchain/registry"
)

func main() {
	var cfgPath string
	var once bool
	flag.StringVar(&cfgPath, "config", defaultConfigPath(), "path to verifierd config json")
	flag.BoolVar(&once, "once", false, "run one poll iteration and wait for any started assignments to finish")
	flag.Parse()

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	client, err := NewChainClient(cfg.GRPCAddr)
	if err != nil {
		log.Fatalf("failed to connect to gRPC: %v", err)
	}
	defer client.Close()

	verifier := registryoffchain.HTTPContentVerifier{Scheme: cfg.VerifyScheme}
	health := newDaemonHealth(time.Duration(cfg.PollIntervalSec) * time.Second)
	agent := &Agent{
		Cfg:      cfg,
		Chain:    client,
		Verifier: verifier,
		Health:   health,
	}

	ctx := context.Background()
	if once {
		if err := agent.PollOnce(ctx); err != nil {
			log.Fatalf("poll failed: %v", err)
		}
		agent.Wait()
		return
	}

	healthServer := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           healthRoutes(cfg, health, agent),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("verifierd health listening on %s", cfg.ListenAddr)
		if err := healthServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("health listen: %v", err)
		}
	}()

	log.Printf("verifierd started (verifier=%s, poll=%ds, listen=%s, commit_start_buffer=%ds, tx_inclusion_timeout=%ds, retry_backoff=%ds)",
		cfg.VerifierAddress, cfg.PollIntervalSec, cfg.ListenAddr, cfg.CommitStartBufferSeconds, cfg.TxInclusionTimeoutSeconds, cfg.RetryBackoffSeconds)
	ticker := time.NewTicker(time.Duration(cfg.PollIntervalSec) * time.Second)
	defer ticker.Stop()
	for {
		health.recordPollAttempt()
		err := agent.PollOnce(ctx)
		health.recordPollResult(err)
		if err != nil {
			log.Printf("poll error: %v", err)
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
		return "offchain/verifierd/config.json"
	}
	return filepath.Join(wd, "offchain", "verifierd", "config.json")
}
