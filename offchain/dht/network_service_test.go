package dht

import (
	"context"
	"testing"
	"time"
)

func TestNetworkServicePublishFindAcrossPeers(t *testing.T) {
	ctx := context.Background()

	primary := newTestNetworkService(t, ctx, nil)
	defer func() {
		if err := primary.Close(); err != nil {
			t.Fatalf("close primary: %v", err)
		}
	}()

	secondary := newTestNetworkService(t, ctx, primary.Addresses())
	defer func() {
		if err := secondary.Close(); err != nil {
			t.Fatalf("close secondary: %v", err)
		}
	}()

	record := Record{
		ID:       "doc-1",
		Vector:   []float64{0.11, 0.42, 0.73},
		Metadata: map[string]string{"source": "primary"},
	}

	if err := primary.Publish("vector-key", record); err != nil {
		t.Fatalf("publish from primary: %v", err)
	}

	results := waitForRecords(t, secondary, "vector-key", 1, 10*time.Second)
	if len(results) != 1 {
		t.Fatalf("expected 1 record, got %d", len(results))
	}
	if results[0].ID != record.ID {
		t.Fatalf("expected record id %s, got %s", record.ID, results[0].ID)
	}
	if results[0].Metadata["source"] != "primary" {
		t.Fatalf("expected metadata to replicate")
	}

	// Update existing record from secondary and ensure it propagates back.
	record.Metadata["source"] = "secondary"
	if err := secondary.Publish("vector-key", record); err != nil {
		t.Fatalf("republish from secondary: %v", err)
	}

	updated := waitForRecords(t, primary, "vector-key", 1, 10*time.Second)
	if updated[0].Metadata["source"] != "secondary" {
		t.Fatalf("expected metadata update to propagate, got %s", updated[0].Metadata["source"])
	}

	// Publish a second record and verify limit handling.
	next := Record{ID: "doc-2", Vector: []float64{0.2, 0.3, 0.4}}
	if err := primary.Publish("vector-key", next); err != nil {
		t.Fatalf("publish second record: %v", err)
	}

	aggregated := waitForRecords(t, secondary, "vector-key", 2, 10*time.Second)
	if len(aggregated) < 2 {
		t.Fatalf("expected at least 2 records, got %d", len(aggregated))
	}

	limited, err := secondary.Find("vector-key", 1)
	if err != nil {
		t.Fatalf("find with limit: %v", err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected limited result to return 1 record, got %d", len(limited))
	}
}

func newTestNetworkService(t *testing.T, ctx context.Context, bootstrap []string) *NetworkService {
	t.Helper()

	cfg := DefaultNetworkConfig()
	cfg.ListenAddrs = []string{"/ip4/127.0.0.1/tcp/0"}
	cfg.BootstrapPeers = append([]string(nil), bootstrap...)
	cfg.RequestTimeout = 5 * time.Second
	svc, err := NewNetworkService(ctx, cfg)
	if err != nil {
		t.Fatalf("create network service: %v", err)
	}
	return svc
}

func waitForRecords(t *testing.T, svc *NetworkService, key string, expected int, timeout time.Duration) []Record {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		records, err := svc.Find(key, expected)
		if err != nil {
			t.Fatalf("find records: %v", err)
		}
		if len(records) >= expected {
			return records
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d records on key %s", expected, key)
	return nil
}
