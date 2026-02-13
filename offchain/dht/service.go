package dht

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"
)

// Config defines runtime parameters for the DHT service.
type Config struct {
	ReplicationFactor int
}

// DefaultConfig returns conservative defaults suitable for tests and local dev.
func DefaultConfig() Config {
	return Config{ReplicationFactor: 3}
}

// Validate ensures the configuration values are sane.
func (c Config) Validate() error {
	if c.ReplicationFactor <= 0 {
		return fmt.Errorf("replication factor must be positive")
	}
	return nil
}

// Record represents a stored vector embedding with optional payload metadata.
type Record struct {
	ID       string            `json:"id"`
	Vector   []float64         `json:"vector"`
	Payload  []byte            `json:"payload"`
	Metadata map[string]string `json:"metadata"`
}

// Validate ensures the record contains the minimum required fields.
func (r Record) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("record id required")
	}
	if len(r.Vector) == 0 {
		return fmt.Errorf("record vector required")
	}
	return nil
}

// Service represents a lightweight DHT node backed by in-memory storage.
type Service struct {
	id    string
	cfg   Config
	mu    sync.RWMutex
	store map[string]map[string]Record // key -> recordID -> record
	peers map[string]*Service
}

// NewService constructs a DHT service with the provided identifier and config.
func NewService(id string, cfg Config) (*Service, error) {
	if id == "" {
		return nil, fmt.Errorf("service id required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Service{
		id:    id,
		cfg:   cfg,
		store: make(map[string]map[string]Record),
		peers: make(map[string]*Service),
	}, nil
}

// ID returns the peer identifier.
func (s *Service) ID() string {
	return s.id
}

// Bootstrap registers the provided peers for routing decisions.
func (s *Service) Bootstrap(peers ...*Service) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range peers {
		if p == nil || p.id == s.id {
			continue
		}
		s.peers[p.id] = p
	}
}

// Publish stores a record under the given key and replicates it across peers.
func (s *Service) Publish(key string, record Record) error {
	if key == "" {
		return fmt.Errorf("key required")
	}
	if err := record.Validate(); err != nil {
		return err
	}

	targets := s.resolvePeers(key)
	for _, target := range targets {
		target.storeRecord(key, record)
	}
	return nil
}

// Find returns records for the key up to the provided limit.
func (s *Service) Find(key string, limit int) ([]Record, error) {
	if key == "" {
		return nil, fmt.Errorf("key required")
	}
	if limit <= 0 {
		limit = s.cfg.ReplicationFactor
	}
	targets := s.resolvePeers(key)
	results := make([]Record, 0, limit)
	seen := make(map[string]struct{})
	for _, target := range targets {
		records := target.loadRecords(key)
		for _, rec := range records {
			if _, ok := seen[rec.ID]; ok {
				continue
			}
			results = append(results, rec)
			seen[rec.ID] = struct{}{}
			if len(results) >= limit {
				return results, nil
			}
		}
	}
	return results, nil
}

func (s *Service) storeRecord(key string, record Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket, ok := s.store[key]
	if !ok {
		bucket = make(map[string]Record)
		s.store[key] = bucket
	}
	// create defensive copy to avoid external mutation
	bucket[record.ID] = cloneRecord(record)
}

func (s *Service) loadRecords(key string) []Record {
	s.mu.RLock()
	bucket := s.store[key]
	s.mu.RUnlock()
	out := make([]Record, 0, len(bucket))
	for _, rec := range bucket {
		out = append(out, cloneRecord(rec))
	}
	return out
}

func (s *Service) resolvePeers(key string) []*Service {
	candidates := s.candidateSet()
	hash := sha256.Sum256([]byte(key))
	sort.Slice(candidates, func(i, j int) bool {
		di := distance(hash[:], candidates[i].id)
		dj := distance(hash[:], candidates[j].id)
		if di == dj {
			return candidates[i].id < candidates[j].id
		}
		return di < dj
	})
	if len(candidates) > s.cfg.ReplicationFactor {
		candidates = candidates[:s.cfg.ReplicationFactor]
	}
	return candidates
}

func (s *Service) candidateSet() []*Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Service, 0, len(s.peers)+1)
	out = append(out, s)
	for _, peer := range s.peers {
		out = append(out, peer)
	}
	return out
}

func distance(hash []byte, peerID string) uint64 {
	ph := sha256.Sum256([]byte(peerID))
	// take XOR distance truncated to uint64
	var v uint64
	for i := 0; i < 8 && i < len(hash); i++ {
		v = (v << 8) | uint64(hash[i]^ph[i])
	}
	return v
}

func cloneRecord(r Record) Record {
	cloned := Record{ID: r.ID}
	if len(r.Vector) > 0 {
		cloned.Vector = append([]float64(nil), r.Vector...)
	}
	if len(r.Payload) > 0 {
		cloned.Payload = append([]byte(nil), r.Payload...)
	}
	if len(r.Metadata) > 0 {
		cloned.Metadata = make(map[string]string, len(r.Metadata))
		for k, v := range r.Metadata {
			cloned.Metadata[k] = v
		}
	}
	return cloned
}
