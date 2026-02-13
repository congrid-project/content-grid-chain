package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPFetcher retrieves remote content via HTTP GET requests.
type HTTPFetcher struct {
	client *http.Client
}

// NewHTTPFetcher constructs an HTTPFetcher with the provided timeout.
func NewHTTPFetcher(timeout time.Duration) *HTTPFetcher {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &HTTPFetcher{client: &http.Client{Timeout: timeout}}
}

// Fetch implements the Fetcher interface using HTTP GET.
func (f *HTTPFetcher) Fetch(ctx context.Context, target string) ([]byte, error) {
	if target == "" {
		return nil, errors.New("target required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http get %s: %w", target, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("http get %s: status %d: %s", target, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if len(data) == 0 {
		return nil, errors.New("empty response body")
	}
	return data, nil
}

// SentenceTransformerConfig controls the behaviour of SentenceTransformerClient.
type SentenceTransformerConfig struct {
	BaseURL   string
	Model     string
	Normalize bool
	Timeout   time.Duration
}

// DefaultSentenceTransformerConfig returns local defaults.
func DefaultSentenceTransformerConfig() SentenceTransformerConfig {
	return SentenceTransformerConfig{
		BaseURL: "http://127.0.0.1:9000",
		Timeout: 15 * time.Second,
	}
}

// Validate ensures configuration values are usable.
func (c SentenceTransformerConfig) Validate() error {
	if c.BaseURL == "" {
		return errors.New("base url required")
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	return nil
}

// SentenceTransformerClient calls a python sentence-transformer service over HTTP.
type SentenceTransformerClient struct {
	cfg    SentenceTransformerConfig
	client *http.Client
}

// NewSentenceTransformerClient constructs a new embedder client.
func NewSentenceTransformerClient(cfg SentenceTransformerConfig) (*SentenceTransformerClient, error) {
	if cfg.BaseURL == "" {
		defaults := DefaultSentenceTransformerConfig()
		cfg.BaseURL = defaults.BaseURL
		if cfg.Timeout == 0 {
			cfg.Timeout = defaults.Timeout
		}
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &SentenceTransformerClient{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}, nil
}

// Embed posts the document to the transformer service and returns the embedding.
func (c *SentenceTransformerClient) Embed(ctx context.Context, content []byte) ([]float64, error) {
	if len(content) == 0 {
		return nil, errors.New("content required")
	}
	payload := map[string]any{
		"text":      string(content),
		"normalize": c.cfg.Normalize,
	}
	if c.cfg.Model != "" {
		payload["model"] = c.cfg.Model
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.BaseURL, "/")+"/embed", bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("build embed request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call sentence transformer: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("embed request failed: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode embed response: %w", err)
	}
	if len(decoded.Embedding) == 0 {
		return nil, errors.New("empty embedding returned")
	}
	return append([]float64(nil), decoded.Embedding...), nil
}

// OllamaZeroShotConfig controls the behaviour of the zero-shot classifier.
type OllamaZeroShotConfig struct {
	Endpoint    string
	Model       string
	Categories  []string
	Timeout     time.Duration
	Temperature float64
}

// DefaultOllamaZeroShotConfig returns defaults for a local Ollama daemon.
func DefaultOllamaZeroShotConfig() OllamaZeroShotConfig {
	return OllamaZeroShotConfig{
		Endpoint:    "http://127.0.0.1:11434",
		Timeout:     30 * time.Second,
		Temperature: 0,
	}
}

// Validate ensures the configuration is usable.
func (c OllamaZeroShotConfig) Validate() error {
	if c.Endpoint == "" {
		return errors.New("endpoint required")
	}
	if c.Model == "" {
		return errors.New("model required")
	}
	if len(c.Categories) == 0 {
		return errors.New("at least one category required")
	}
	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	return nil
}

// OllamaZeroShotClassifier performs zero-shot classification via an Ollama model.
type OllamaZeroShotClassifier struct {
	cfg    OllamaZeroShotConfig
	client *http.Client
}

// NewOllamaZeroShotClassifier constructs a classifier backed by a local Ollama daemon.
func NewOllamaZeroShotClassifier(cfg OllamaZeroShotConfig) (*OllamaZeroShotClassifier, error) {
	defaults := DefaultOllamaZeroShotConfig()
	if cfg.Endpoint == "" {
		cfg.Endpoint = defaults.Endpoint
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaults.Timeout
	}
	if cfg.Temperature == 0 {
		cfg.Temperature = defaults.Temperature
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	trimmedCats := make([]string, 0, len(cfg.Categories))
	for _, cat := range cfg.Categories {
		cat = strings.TrimSpace(cat)
		if cat == "" {
			continue
		}
		trimmedCats = append(trimmedCats, cat)
	}
	if len(trimmedCats) == 0 {
		return nil, errors.New("categories cannot be empty after trimming")
	}
	cfg.Categories = trimmedCats

	return &OllamaZeroShotClassifier{
		cfg:    cfg,
		client: &http.Client{Timeout: cfg.Timeout},
	}, nil
}

// Classify sends the content to the Ollama API and extracts the predicted label.
func (c *OllamaZeroShotClassifier) Classify(ctx context.Context, content []byte) (string, error) {
	if len(content) == 0 {
		return "", errors.New("content required")
	}
	prompt := c.buildPrompt(string(content))
	reqBody := map[string]any{
		"model":  c.cfg.Model,
		"prompt": prompt,
		"stream": false,
		"options": map[string]any{
			"temperature": c.cfg.Temperature,
		},
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("encode ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.Endpoint, "/")+"/api/generate", bytes.NewReader(buf))
	if err != nil {
		return "", fmt.Errorf("build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("call ollama: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decode ollama response: %w", err)
	}
	label := c.extractLabel(decoded.Response)
	if label == "" {
		return "", fmt.Errorf("ollama response did not contain a supported category: %s", decoded.Response)
	}
	return label, nil
}

func (c *OllamaZeroShotClassifier) buildPrompt(content string) string {
	categories := strings.Join(c.cfg.Categories, ", ")
	builder := strings.Builder{}
	builder.WriteString("You are a classification model. Given the content below, select exactly one label from the list.\n")
	builder.WriteString("Return only the label text with no punctuation or explanation.\n\n")
	builder.WriteString("Labels: ")
	builder.WriteString(categories)
	builder.WriteString("\n\nContent:\n")
	if len(content) > 4000 {
		builder.WriteString(content[:4000])
		builder.WriteString("...\n")
	} else {
		builder.WriteString(content)
	}
	builder.WriteString("\nLabel:")
	return builder.String()
}

func (c *OllamaZeroShotClassifier) extractLabel(response string) string {
	cleaned := strings.TrimSpace(response)
	cleaned = strings.Trim(cleaned, "\"'` “”")
	cleanedLower := strings.ToLower(cleaned)
	for _, cat := range c.cfg.Categories {
		if strings.EqualFold(cleaned, cat) {
			return cat
		}
	}
	for _, cat := range c.cfg.Categories {
		if strings.Contains(cleanedLower, strings.ToLower(cat)) {
			return cat
		}
	}
	return ""
}
