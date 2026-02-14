package main

import (
	"sync"
	"time"
)

type PublisherDoc struct {
	Domain    string    `json:"domain"`
	FetchedAt time.Time `json:"fetched_at"`
	SourceURL string    `json:"source_url"`
	Status    string    `json:"status"` // ok|error
	Error     string    `json:"error,omitempty"`
	Title     string    `json:"title,omitempty"`

	// Large fields: typically redacted from API responses unless requested.
	Markdown  string    `json:"markdown,omitempty"`
	Embedding []float64 `json:"embedding,omitempty"`

	// Storage hint: if Chroma is enabled, embeddings may be persisted externally and left empty here.
	EmbeddingExternal bool `json:"embedding_external,omitempty"`

	EmbeddingDim        int    `json:"embedding_dim,omitempty"`
	EmbeddingNormalized bool   `json:"embedding_normalized,omitempty"`
	Signature           string `json:"signature,omitempty"`      // hex
	SignatureBits       int    `json:"signature_bits,omitempty"` // e.g. 128
	SignatureAlgo       string `json:"signature_algo,omitempty"` // e.g. fold-sign-v1
	CongridLinks        int    `json:"congrid_links"`
	Wallet              string `json:"wallet,omitempty"`
	BodySHA256          string `json:"body_sha256,omitempty"`
	ResponseBytes       int    `json:"response_bytes,omitempty"`
}

type Store struct {
	mu   sync.RWMutex
	docs map[string]PublisherDoc
}

func NewStore() *Store {
	return &Store{docs: make(map[string]PublisherDoc)}
}

func (s *Store) Put(doc PublisherDoc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[doc.Domain] = doc
}

func (s *Store) Get(domain string) (PublisherDoc, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.docs[domain]
	return d, ok
}

func (s *Store) Delete(domain string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, domain)
}

func (s *Store) List() []PublisherDoc {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PublisherDoc, 0, len(s.docs))
	for _, d := range s.docs {
		out = append(out, d)
	}
	return out
}
