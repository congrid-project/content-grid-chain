package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPFetcherSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("payload"))
	}))
	defer srv.Close()

	fetcher := NewHTTPFetcher(2 * time.Second)
	data, err := fetcher.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected fetch error: %v", err)
	}
	if string(data) != "payload" {
		t.Fatalf("expected payload body, got %s", string(data))
	}
}

func TestHTTPFetcherStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer srv.Close()

	fetcher := NewHTTPFetcher(2 * time.Second)
	if _, err := fetcher.Fetch(context.Background(), srv.URL); err == nil {
		t.Fatalf("expected error for non-200 response")
	}
}

func TestSentenceTransformerClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embed" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode payload: %v", err)
		}
		if payload["text"].(string) != "hello" {
			t.Fatalf("unexpected text payload: %v", payload["text"])
		}
		resp := map[string]any{"embedding": []float64{0.1, 0.2, 0.3}}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	client, err := NewSentenceTransformerClient(SentenceTransformerConfig{
		BaseURL: srv.URL,
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	vec, err := client.Embed(context.Background(), []byte("hello"))
	if err != nil {
		t.Fatalf("unexpected embed error: %v", err)
	}
	if len(vec) != 3 || vec[0] != 0.1 {
		t.Fatalf("unexpected embedding: %#v", vec)
	}
}

func TestOllamaZeroShotClassifier(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["model"].(string) != "llama3" {
			t.Fatalf("unexpected model: %v", req["model"])
		}
		resp := map[string]any{
			"response": "The best label is Positive",
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer srv.Close()

	classifier, err := NewOllamaZeroShotClassifier(OllamaZeroShotConfig{
		Endpoint:   srv.URL,
		Model:      "llama3",
		Categories: []string{"Positive", "Negative"},
		Timeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	label, err := classifier.Classify(context.Background(), []byte("sample input"))
	if err != nil {
		t.Fatalf("unexpected classify error: %v", err)
	}
	if label != "Positive" {
		t.Fatalf("expected Positive label, got %s", label)
	}
}

func TestOllamaZeroShotClassifierNoMatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"response": "I cannot decide"}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	classifier, err := NewOllamaZeroShotClassifier(OllamaZeroShotConfig{
		Endpoint:   srv.URL,
		Model:      "llama3",
		Categories: []string{"Positive", "Negative"},
		Timeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	if _, err := classifier.Classify(context.Background(), []byte("sample")); err == nil {
		t.Fatalf("expected error when response lacks category")
	}
}
