package dht

import (
	"fmt"
	"testing"
)

func TestNewServiceValidation(t *testing.T) {
	if _, err := NewService("", DefaultConfig()); err == nil {
		t.Fatalf("expected error for empty service id")
	}

	cfg := DefaultConfig()
	cfg.ReplicationFactor = 0
	if _, err := NewService("node", cfg); err == nil {
		t.Fatalf("expected error for invalid replication factor")
	}
}

func TestPublishAndFind(t *testing.T) {
	cfg := DefaultConfig()
	nodes := make([]*Service, 0, 4)
	for i := 0; i < 4; i++ {
		svc, err := NewService(testPeerID(i), cfg)
		if err != nil {
			t.Fatalf("unexpected error creating service: %v", err)
		}
		nodes = append(nodes, svc)
	}

	for _, node := range nodes {
		peers := make([]*Service, 0, len(nodes)-1)
		for _, other := range nodes {
			if other.ID() == node.ID() {
				continue
			}
			peers = append(peers, other)
		}
		node.Bootstrap(peers...)
	}

	record := Record{
		ID:     "doc-1",
		Vector: []float64{0.1, 0.2, 0.3},
		Metadata: map[string]string{
			"url": "https://example.com",
		},
	}

	if err := nodes[0].Publish("hash-key", record); err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}

	results, err := nodes[1].Find("hash-key", 5)
	if err != nil {
		t.Fatalf("unexpected find error: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected at least one record")
	}
	if results[0].ID != record.ID {
		t.Fatalf("expected record id %s got %s", record.ID, results[0].ID)
	}
	if results[0].Metadata["url"] != "https://example.com" {
		t.Fatalf("expected metadata to replicate")
	}
}

func TestFindLimitAndDedup(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ReplicationFactor = 2
	a, _ := NewService("peer-a", cfg)
	b, _ := NewService("peer-b", cfg)
	c, _ := NewService("peer-c", cfg)

	a.Bootstrap(b, c)
	b.Bootstrap(a, c)
	c.Bootstrap(a, b)

	rec1 := Record{ID: "r1", Vector: []float64{1}}
	rec2 := Record{ID: "r2", Vector: []float64{2}}

	if err := a.Publish("key", rec1); err != nil {
		t.Fatalf("publish r1: %v", err)
	}
	if err := b.Publish("key", rec2); err != nil {
		t.Fatalf("publish r2: %v", err)
	}

	results, err := c.Find("key", 1)
	if err != nil {
		t.Fatalf("find error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result with limit 1, got %d", len(results))
	}

	results, err = c.Find("key", 5)
	if err != nil {
		t.Fatalf("find error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 unique results, got %d", len(results))
	}
	ids := make(map[string]struct{})
	for _, r := range results {
		ids[r.ID] = struct{}{}
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 unique record ids, got %v", ids)
	}
}

func testPeerID(i int) string {
	return fmt.Sprintf("peer-%d", i)
}
