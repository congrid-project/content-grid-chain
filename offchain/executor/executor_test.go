package executor

import (
	"context"
	"testing"

	"content-grid-chain/offchain/dht"
)

func TestExecutorSuccess(t *testing.T) {
	store, err := dht.NewService("peer-1", dht.DefaultConfig())
	if err != nil {
		t.Fatalf("unexpected error creating dht service: %v", err)
	}

	exec, err := NewExecutor(DefaultConfig(), store, fetcherStub{content: []byte("hello world")}, embedderStub{vector: []float64{0.15, 0.2, 0.3, 0.4}}, classifierStub{label: "safe"})
	if err != nil {
		t.Fatalf("unexpected error constructing executor: %v", err)
	}

	task := Task{ID: "task-123", URL: "https://example.com"}
	res, err := exec.Execute(context.Background(), task)
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}

	if res.Task.ID != task.ID {
		t.Fatalf("expected task id to round-trip")
	}
	if res.Classification != "safe" {
		t.Fatalf("expected classification to be safe, got %s", res.Classification)
	}
	if res.DHTKey == "" || res.RecordID == "" {
		t.Fatalf("expected non-empty DHT key and record id")
	}

	stored, err := store.Find(res.DHTKey, 5)
	if err != nil {
		t.Fatalf("unexpected find error: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("expected one stored record, got %d", len(stored))
	}
	if stored[0].Metadata["task_id"] != task.ID {
		t.Fatalf("expected metadata to include task id")
	}
	if stored[0].Metadata["classification"] != "safe" {
		t.Fatalf("expected classification in metadata")
	}
}

func TestExecutorWithoutClassifier(t *testing.T) {
	store, _ := dht.NewService("peer-1", dht.DefaultConfig())
	exec, err := NewExecutor(DefaultConfig(), store, fetcherStub{content: []byte("body")}, embedderStub{vector: []float64{0.5}}, nil)
	if err != nil {
		t.Fatalf("unexpected err constructing executor: %v", err)
	}
	res, err := exec.Execute(context.Background(), Task{ID: "task", URL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected execute error: %v", err)
	}
	if res.Classification != "" {
		t.Fatalf("expected empty classification when classifier is nil")
	}
}

func TestNewExecutorValidation(t *testing.T) {
	store, _ := dht.NewService("peer-1", dht.DefaultConfig())
	badCfg := DefaultConfig()
	badCfg.LSHPrecision = 0
	if _, err := NewExecutor(badCfg, store, fetcherStub{}, embedderStub{vector: []float64{1}}, nil); err == nil {
		t.Fatalf("expected error for invalid config")
	}
	if _, err := NewExecutor(DefaultConfig(), nil, fetcherStub{}, embedderStub{vector: []float64{1}}, nil); err == nil {
		t.Fatalf("expected error for nil store")
	}
}

func TestExecutePropagatesFetcherError(t *testing.T) {
	store, _ := dht.NewService("peer-1", dht.DefaultConfig())
	exec, err := NewExecutor(DefaultConfig(), store, fetcherStub{err: context.DeadlineExceeded}, embedderStub{vector: []float64{1}}, nil)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	if _, err := exec.Execute(context.Background(), Task{ID: "task", URL: "https://example.com"}); err == nil {
		t.Fatalf("expected execute to return fetcher error")
	}
}

func TestTaskValidate(t *testing.T) {
	if err := (Task{}).Validate(); err == nil {
		t.Fatalf("expected error for missing fields")
	}
	if err := (Task{ID: "id", URL: "not-a-url"}).Validate(); err == nil {
		t.Fatalf("expected error for invalid url")
	}
	if err := (Task{ID: "id", URL: "https://example.com"}).Validate(); err != nil {
		t.Fatalf("expected valid task: %v", err)
	}
}

type fetcherStub struct {
	content []byte
	err     error
}

func (f fetcherStub) Fetch(ctx context.Context, target string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return append([]byte(nil), f.content...), nil
}

type embedderStub struct {
	vector []float64
	err    error
}

func (e embedderStub) Embed(ctx context.Context, content []byte) ([]float64, error) {
	if e.err != nil {
		return nil, e.err
	}
	return append([]float64(nil), e.vector...), nil
}

type classifierStub struct {
	label string
	err   error
}

func (c classifierStub) Classify(ctx context.Context, content []byte) (string, error) {
	if c.err != nil {
		return "", c.err
	}
	return c.label, nil
}
