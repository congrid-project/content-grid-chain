package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	// Publishers is an optional static list of publisher domains to index (may include ports).
	//
	// In production you typically set chain_grpc_addr and let indexerd discover
	// publishers from the chain.
	Publishers []string `json:"publishers"`

	// ChainGRPCAddr enables chain-driven discovery of publishers via the registry module.
	// Example: "127.0.0.1:9090".
	ChainGRPCAddr       string `json:"chain_grpc_addr"`
	ChainTimeoutSeconds int    `json:"chain_timeout_seconds"`
	ChainPageLimit      uint64 `json:"chain_page_limit"`

	ListenAddr string `json:"listen_addr"`

	// Chroma vector DB endpoint used for persistent storage + similarity search.
	// Required for embedding generation and storage.
	// Example: "http://127.0.0.1:8000".
	ChromaBaseURL string `json:"chroma_base_url"`
	// Chroma collection name (default: "publishers").
	ChromaCollection string `json:"chroma_collection"`

	// SignatureBits controls the size of the compact similarity signature returned
	// by the API. Must be a multiple of 8.
	SignatureBits int `json:"signature_bits"`

	FetchTimeoutSeconds  int   `json:"fetch_timeout_seconds"`
	IndexIntervalMinutes int   `json:"index_interval_minutes"`
	MaxBodyBytes         int64 `json:"max_body_bytes"`
}

func loadConfig(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return Config{}, err
	}
	cfg.normalize()
	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) normalize() {
	for i := range c.Publishers {
		c.Publishers[i] = strings.TrimSpace(strings.ToLower(c.Publishers[i]))
	}
	c.ChainGRPCAddr = strings.TrimSpace(c.ChainGRPCAddr)
	c.ListenAddr = strings.TrimSpace(c.ListenAddr)
	c.ChromaBaseURL = strings.TrimSpace(c.ChromaBaseURL)
	c.ChromaCollection = strings.TrimSpace(c.ChromaCollection)
}

func (c *Config) applyDefaults() {
	if c.ListenAddr == "" {
		c.ListenAddr = "127.0.0.1:9100"
	}
	if c.FetchTimeoutSeconds <= 0 {
		c.FetchTimeoutSeconds = 10
	}
	if c.IndexIntervalMinutes <= 0 {
		c.IndexIntervalMinutes = 60
	}
	if c.MaxBodyBytes <= 0 {
		c.MaxBodyBytes = 1 << 20
	}
	if c.ChainTimeoutSeconds <= 0 {
		c.ChainTimeoutSeconds = 10
	}
	if c.ChainPageLimit == 0 {
		c.ChainPageLimit = 200
	}
	if c.SignatureBits <= 0 {
		c.SignatureBits = 128
	}
	if c.ChromaCollection == "" {
		c.ChromaCollection = "publishers"
	}
}

func (c Config) Validate() error {
	if len(c.Publishers) == 0 && c.ChainGRPCAddr == "" {
		return fmt.Errorf("either publishers or chain_grpc_addr required")
	}
	if c.ListenAddr == "" {
		return fmt.Errorf("listen_addr required")
	}
	if c.ChromaBaseURL == "" {
		return fmt.Errorf("chroma_base_url required")
	}
	if c.ChromaCollection == "" {
		return fmt.Errorf("chroma_collection required")
	}
	if c.FetchTimeoutSeconds <= 0 {
		return fmt.Errorf("fetch_timeout_seconds must be positive")
	}
	if c.IndexIntervalMinutes <= 0 {
		return fmt.Errorf("index_interval_minutes must be positive")
	}
	if c.MaxBodyBytes <= 0 {
		return fmt.Errorf("max_body_bytes must be positive")
	}
	if c.ChainGRPCAddr != "" {
		if c.ChainTimeoutSeconds <= 0 {
			return fmt.Errorf("chain_timeout_seconds must be positive")
		}
		if c.ChainPageLimit == 0 {
			return fmt.Errorf("chain_page_limit must be positive")
		}
	}
	if c.SignatureBits <= 0 {
		return fmt.Errorf("signature_bits must be positive")
	}
	if c.SignatureBits%8 != 0 {
		return fmt.Errorf("signature_bits must be a multiple of 8")
	}
	return nil
}

func (c Config) FetchTimeout() time.Duration {
	return time.Duration(c.FetchTimeoutSeconds) * time.Second
}

func (c Config) IndexInterval() time.Duration {
	return time.Duration(c.IndexIntervalMinutes) * time.Minute
}

func (c Config) ChainTimeout() time.Duration {
	return time.Duration(c.ChainTimeoutSeconds) * time.Second
}
