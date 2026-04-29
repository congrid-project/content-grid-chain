package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	GRPCAddr              string `json:"grpc_addr"`
	DrandAPIBaseURL       string `json:"drand_api_base_url"`
	DrandChainHash        string `json:"drand_chain_hash"`
	PollIntervalSec       int    `json:"poll_interval_seconds"`
	MinSubmitIntervalSec  int    `json:"min_submit_interval_seconds"`
	RequestTimeoutSec     int    `json:"request_timeout_seconds"`
	RetryBackoffSec       int    `json:"retry_backoff_seconds"`
	TxInclusionTimeoutSec int    `json:"tx_inclusion_timeout_seconds"`
	MaxSubmitRetries      int    `json:"max_submit_retries"`

	Submit SubmitConfig `json:"submit"`
}

type SubmitConfig struct {
	Binary         string  `json:"binary"`
	ChainID        string  `json:"chain_id"`
	Node           string  `json:"node"`
	From           string  `json:"from"`
	KeyringBackend string  `json:"keyring_backend"`
	KeyringDir     string  `json:"keyring_dir"`
	KeyringPassEnv string  `json:"keyring_passphrase_env"`
	Home           string  `json:"home"`
	Gas            string  `json:"gas"`
	GasAdjustment  float64 `json:"gas_adjustment"`
	Fees           string  `json:"fees"`
	GasPrices      string  `json:"gas_prices"`
	BroadcastMode  string  `json:"broadcast_mode"`
	Yes            bool    `json:"yes"`
}

func loadConfig(path string) (Config, error) {
	bz, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(bz, &cfg); err != nil {
		return Config{}, err
	}
	cfg.normalize()
	cfg.applyDefaults()
	return cfg, nil
}

func (c *Config) normalize() {
	c.GRPCAddr = strings.TrimSpace(c.GRPCAddr)
	c.DrandAPIBaseURL = strings.TrimSpace(c.DrandAPIBaseURL)
	c.DrandChainHash = strings.TrimSpace(c.DrandChainHash)
	c.Submit.Binary = strings.TrimSpace(c.Submit.Binary)
	c.Submit.ChainID = strings.TrimSpace(c.Submit.ChainID)
	c.Submit.Node = strings.TrimSpace(c.Submit.Node)
	c.Submit.From = strings.TrimSpace(c.Submit.From)
	c.Submit.KeyringBackend = strings.TrimSpace(c.Submit.KeyringBackend)
	c.Submit.KeyringDir = strings.TrimSpace(c.Submit.KeyringDir)
	c.Submit.KeyringPassEnv = strings.TrimSpace(c.Submit.KeyringPassEnv)
	c.Submit.Home = strings.TrimSpace(c.Submit.Home)
	c.Submit.Gas = strings.TrimSpace(c.Submit.Gas)
	c.Submit.Fees = strings.TrimSpace(c.Submit.Fees)
	c.Submit.GasPrices = strings.TrimSpace(c.Submit.GasPrices)
	c.Submit.BroadcastMode = strings.TrimSpace(c.Submit.BroadcastMode)
}

func (c *Config) applyDefaults() {
	if c.GRPCAddr == "" {
		c.GRPCAddr = "127.0.0.1:9090"
	}
	if c.DrandAPIBaseURL == "" {
		c.DrandAPIBaseURL = "https://api.drand.sh"
	}
	if c.PollIntervalSec <= 0 {
		c.PollIntervalSec = 60
	}
	if c.MinSubmitIntervalSec <= 0 {
		c.MinSubmitIntervalSec = 300
	}
	if c.RequestTimeoutSec <= 0 {
		c.RequestTimeoutSec = 10
	}
	if c.RetryBackoffSec <= 0 {
		c.RetryBackoffSec = 30
	}
	if c.TxInclusionTimeoutSec <= 0 {
		c.TxInclusionTimeoutSec = 120
	}
	if c.MaxSubmitRetries <= 0 {
		c.MaxSubmitRetries = 1
	}
	if c.Submit.Binary == "" {
		c.Submit.Binary = "content-grid-d"
	}
	if c.Submit.Node == "" {
		c.Submit.Node = "tcp://localhost:26657"
	}
	if c.Submit.KeyringBackend == "" {
		c.Submit.KeyringBackend = "os"
	}
	if c.Submit.Gas == "" {
		c.Submit.Gas = "220000"
	}
	if c.Submit.GasAdjustment <= 0 {
		c.Submit.GasAdjustment = 1
	}
	if c.Submit.BroadcastMode == "" {
		c.Submit.BroadcastMode = "sync"
	}
}

func (c Config) Validate() error {
	if c.GRPCAddr == "" {
		return fmt.Errorf("grpc_addr required")
	}
	if c.DrandChainHash == "" {
		return fmt.Errorf("drand_chain_hash required")
	}
	if c.Submit.ChainID == "" {
		return fmt.Errorf("submit.chain_id required")
	}
	if c.Submit.From == "" {
		return fmt.Errorf("submit.from required")
	}
	return nil
}
