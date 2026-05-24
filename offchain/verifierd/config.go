package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	GRPCAddr                  string `json:"grpc_addr"`
	ListenAddr                string `json:"listen_addr"`
	VerifierAddress           string `json:"verifier_address"`
	StateDir                  string `json:"state_dir"`
	PollIntervalSec           int    `json:"poll_interval_seconds"`
	VerifyScheme              string `json:"verify_scheme"`
	CommitStartBufferSeconds  int64  `json:"commit_start_buffer_seconds"`
	CommitWindowSeconds       int64  `json:"commit_window_seconds"`
	RoundIntervalSeconds      int64  `json:"round_interval_seconds"`
	AssignmentDelayMaxSeconds int64  `json:"assignment_delay_max_seconds"`
	DisableAssignmentCheck    bool   `json:"disable_assignment_check"`
	RetryBackoffSeconds       int    `json:"retry_backoff_seconds"`
	TxInclusionTimeoutSeconds int    `json:"tx_inclusion_timeout_seconds"`

	// Similarity backend (optional): indexerd endpoint providing /v1/similar.
	// Example: "http://127.0.0.1:9100".
	IndexerdBaseURL string `json:"indexerd_base_url"`

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
	c.GRPCAddr = strings.TrimSpace(c.GRPCAddr)
	c.ListenAddr = strings.TrimSpace(c.ListenAddr)
	c.VerifierAddress = strings.TrimSpace(c.VerifierAddress)
	c.StateDir = strings.TrimSpace(c.StateDir)
	c.VerifyScheme = strings.TrimSpace(c.VerifyScheme)
	c.IndexerdBaseURL = strings.TrimSpace(c.IndexerdBaseURL)
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
	if c.ListenAddr == "" {
		c.ListenAddr = "127.0.0.1:9200"
	}
	if c.PollIntervalSec <= 0 {
		c.PollIntervalSec = 15
	}
	if c.VerifyScheme == "" {
		c.VerifyScheme = "https"
	}
	if c.CommitStartBufferSeconds <= 0 {
		c.CommitStartBufferSeconds = 15
	}
	if c.CommitWindowSeconds <= 0 {
		c.CommitWindowSeconds = 300
	}
	if c.RoundIntervalSeconds <= 0 {
		c.RoundIntervalSeconds = 3600
	}
	if c.AssignmentDelayMaxSeconds <= 0 {
		c.AssignmentDelayMaxSeconds = c.RoundIntervalSeconds
	}
	if c.AssignmentDelayMaxSeconds > c.RoundIntervalSeconds {
		c.AssignmentDelayMaxSeconds = c.RoundIntervalSeconds
	}
	if c.RetryBackoffSeconds <= 0 {
		c.RetryBackoffSeconds = 30
	}
	if c.TxInclusionTimeoutSeconds <= 0 {
		c.TxInclusionTimeoutSeconds = 120
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
		c.Submit.Gas = "200000"
	}
	if c.Submit.GasAdjustment <= 0 {
		c.Submit.GasAdjustment = 1
	}
	if c.Submit.BroadcastMode == "" {
		c.Submit.BroadcastMode = "sync"
	}
	if c.StateDir == "" {
		if c.Submit.Home != "" {
			c.StateDir = filepath.Join(c.Submit.Home, "verifierd-state")
		} else {
			c.StateDir = ".verifierd-state"
		}
	}
}

func (c Config) Validate() error {
	if c.GRPCAddr == "" {
		return fmt.Errorf("grpc_addr required")
	}
	if c.ListenAddr == "" {
		return fmt.Errorf("listen_addr required")
	}
	if c.VerifierAddress == "" {
		return fmt.Errorf("verifier_address required")
	}
	if c.CommitWindowSeconds <= 0 {
		return fmt.Errorf("commit_window_seconds must be positive")
	}
	if c.CommitStartBufferSeconds < 0 {
		return fmt.Errorf("commit_start_buffer_seconds must be non-negative")
	}
	if c.CommitStartBufferSeconds >= c.CommitWindowSeconds {
		return fmt.Errorf("commit_start_buffer_seconds must be smaller than commit_window_seconds")
	}
	if c.RetryBackoffSeconds <= 0 {
		return fmt.Errorf("retry_backoff_seconds must be positive")
	}
	if c.TxInclusionTimeoutSeconds <= 0 {
		return fmt.Errorf("tx_inclusion_timeout_seconds must be positive")
	}
	if c.Submit.ChainID == "" {
		return fmt.Errorf("submit.chain_id required")
	}
	if c.Submit.From == "" {
		return fmt.Errorf("submit.from required")
	}
	return nil
}
