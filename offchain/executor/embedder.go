package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// SentenceTransformerConfig configures the embedding client.
type SentenceTransformerConfig struct {
	BaseURL   string
	Normalize bool
	Timeout   time.Duration
	Model     string
}

// SentenceTransformerClient calls the sentence transformer HTTP service.
type SentenceTransformerClient struct {
	baseURL   string
	normalize bool
	model     string
	client    *http.Client
}

// NewSentenceTransformerClient initializes a client for the embedding service.
func NewSentenceTransformerClient(cfg SentenceTransformerConfig) (*SentenceTransformerClient, error) {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		return nil, fmt.Errorf("embedder base url required")
	}
	baseURL = strings.TrimRight(baseURL, "/")
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &SentenceTransformerClient{
		baseURL:   baseURL,
		normalize: cfg.Normalize,
		model:     strings.TrimSpace(cfg.Model),
		client:    &http.Client{Timeout: timeout},
	}, nil
}

// Embed returns the embedding vector for the provided document.
func (c *SentenceTransformerClient) Embed(ctx context.Context, doc []byte) ([]float64, error) {
	if c == nil {
		return nil, fmt.Errorf("embedder not initialized")
	}
	text := strings.TrimSpace(string(doc))
	if text == "" {
		return nil, fmt.Errorf("empty document")
	}

	payload := map[string]any{
		"text":      text,
		"normalize": c.normalize,
	}
	if c.model != "" {
		payload["model"] = c.model
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	endpoint := c.baseURL + "/embed"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed request: %w", err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("embed failed status=%d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var decoded struct {
		Embedding []float64 `json:"embedding"`
		Dim       int       `json:"dim"`
		Model     string    `json:"model"`
		Normalize bool      `json:"normalize"`
	}
	if err := json.Unmarshal(respBody, &decoded); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(decoded.Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding")
	}
	if decoded.Dim > 0 && decoded.Dim != len(decoded.Embedding) {
		return nil, fmt.Errorf("embedding dimension mismatch: expected %d got %d", decoded.Dim, len(decoded.Embedding))
	}
	return decoded.Embedding, nil
}
