package executor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strings"

	"content-grid-chain/offchain/dht"
)

// VectorStore abstracts the subset of DHT operations used by the executor.
type VectorStore interface {
	Publish(key string, record dht.Record) error
}

// Fetcher retrieves raw content for a given URL.
type Fetcher interface {
	Fetch(ctx context.Context, target string) ([]byte, error)
}

// Embedder converts raw content into a vector embedding.
type Embedder interface {
	Embed(ctx context.Context, content []byte) ([]float64, error)
}

// Classifier performs lightweight classification on the content.
type Classifier interface {
	Classify(ctx context.Context, content []byte) (string, error)
}

// Task represents an off-chain assignment received from the chain.
type Task struct {
	ID  string
	URL string
}

// Validate ensures the task fields are present and well-formed.
func (t Task) Validate() error {
	if t.ID == "" {
		return errors.New("task id required")
	}
	if t.URL == "" {
		return errors.New("task URL required")
	}
	if _, err := url.ParseRequestURI(t.URL); err != nil {
		return fmt.Errorf("invalid task URL: %w", err)
	}
	return nil
}

// Result captures the executor output that will be committed back on-chain.
type Result struct {
	Task           Task
	ContentHash    string
	Vector         []float64
	Classification string
	DHTKey         string
	RecordID       string
}

// Config controls executor behaviour.
type Config struct {
	LSHPrecision    int
	MaxPayloadBytes int
}

// DefaultConfig returns tuned defaults for development usage.
func DefaultConfig() Config {
	return Config{
		LSHPrecision:    4,
		MaxPayloadBytes: 4096,
	}
}

// Validate ensures config values are within sane bounds.
func (c Config) Validate() error {
	if c.LSHPrecision <= 0 {
		return fmt.Errorf("lsh precision must be positive")
	}
	if c.MaxPayloadBytes <= 0 {
		return fmt.Errorf("max payload bytes must be positive")
	}
	return nil
}

// Executor orchestrates fetch -> embed -> classify -> publish flow for tasks.
type Executor struct {
	cfg        Config
	store      VectorStore
	fetcher    Fetcher
	embedder   Embedder
	classifier Classifier
}

// NewExecutor constructs an executor with validated config and dependencies.
func NewExecutor(cfg Config, store VectorStore, fetcher Fetcher, embedder Embedder, classifier Classifier) (*Executor, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if store == nil {
		return nil, errors.New("store required")
	}
	if fetcher == nil {
		return nil, errors.New("fetcher required")
	}
	if embedder == nil {
		return nil, errors.New("embedder required")
	}
	return &Executor{
		cfg:        cfg,
		store:      store,
		fetcher:    fetcher,
		embedder:   embedder,
		classifier: classifier,
	}, nil
}

// Execute runs the task lifecycle and publishes the resulting record into the DHT.
func (e *Executor) Execute(ctx context.Context, task Task) (Result, error) {
	if err := task.Validate(); err != nil {
		return Result{}, err
	}

	raw, err := e.fetcher.Fetch(ctx, task.URL)
	if err != nil {
		return Result{}, fmt.Errorf("fetch content: %w", err)
	}
	if len(raw) == 0 {
		return Result{}, errors.New("empty content fetched")
	}

	processedDoc, procErr := PreprocessDocument(raw)
	embedInput := raw
	if procErr == nil && processedDoc.Markdown != "" {
		embedInput = buildEmbeddingInput(processedDoc)
	}
	if len(embedInput) == 0 {
		embedInput = raw
	}

	vector, err := e.embedder.Embed(ctx, embedInput)
	if err != nil {
		return Result{}, fmt.Errorf("embed content: %w", err)
	}
	if len(vector) == 0 {
		return Result{}, errors.New("embedder returned empty vector")
	}

	classification := ""
	if e.classifier != nil {
		classification, err = e.classifier.Classify(ctx, embedInput)
		if err != nil {
			return Result{}, fmt.Errorf("classify content: %w", err)
		}
	}

	contentHash := hashBytes(raw)
	dhtKey := e.lshKey(vector)
	recordID := fmt.Sprintf("%s:%s", task.ID, contentHash[:16])

	metadata := map[string]string{
		"task_id": task.ID,
		"url":     task.URL,
		"hash":    contentHash,
	}
	if classification != "" {
		metadata["classification"] = classification
	}
	if processedDoc.Title != "" {
		metadata["title"] = processedDoc.Title
	}
	metadata["format"] = "markdown"

	payload := embedInput
	if len(payload) > e.cfg.MaxPayloadBytes {
		payload = payload[:e.cfg.MaxPayloadBytes]
	}

	record := dht.Record{
		ID:       recordID,
		Vector:   append([]float64(nil), vector...),
		Payload:  append([]byte(nil), payload...),
		Metadata: metadata,
	}

	if err := e.store.Publish(dhtKey, record); err != nil {
		return Result{}, fmt.Errorf("publish record: %w", err)
	}

	return Result{
		Task:           task,
		ContentHash:    contentHash,
		Vector:         append([]float64(nil), vector...),
		Classification: classification,
		DHTKey:         dhtKey,
		RecordID:       recordID,
	}, nil
}

func (e *Executor) lshKey(vector []float64) string {
	precision := e.cfg.LSHPrecision
	if precision > len(vector) {
		precision = len(vector)
	}
	builder := strings.Builder{}
	// quantise the first precision dimensions to lock vectors into shared buckets
	for i := 0; i < precision; i++ {
		q := int(math.Round(vector[i] * 100))
		builder.WriteString(fmt.Sprintf("%d|", q))
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:8])
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
