package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Server struct {
	Cfg     Config
	Store   *Store
	Indexer *Indexer
	Chroma  *ChromaClient
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/v1/publishers", s.handleListPublishers)
	mux.HandleFunc("/v1/publishers/", s.handleGetPublisher)
	mux.HandleFunc("/v1/index", s.handleIndexNow)
	mux.HandleFunc("/v1/query", s.handleQuery)
	mux.HandleFunc("/v1/similar", s.handleSimilar)
	return mux
}

func (s *Server) handleListPublishers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	verbose := parseBoolQuery(r, "verbose")

	docs := s.Store.List()
	sort.Slice(docs, func(i, j int) bool { return docs[i].Domain < docs[j].Domain })
	if !verbose {
		for i := range docs {
			docs[i].Markdown = ""
			docs[i].Embedding = nil
		}
	}
	writeJSON(w, docs)
}

func (s *Server) handleGetPublisher(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	verbose := parseBoolQuery(r, "verbose")

	domain := strings.TrimPrefix(r.URL.Path, "/v1/publishers/")
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	doc, ok := s.Store.Get(domain)
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	if !verbose {
		doc.Markdown = ""
		doc.Embedding = nil
	}
	writeJSON(w, doc)
}

func (s *Server) handleIndexNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	go s.Indexer.IndexAll(r.Context())
	writeJSON(w, map[string]any{"status": "scheduled"})
}

type queryReq struct {
	Text  string `json:"text"`
	Limit int    `json:"limit"`
}

type queryHit struct {
	Domain    string  `json:"domain"`
	Score     float64 `json:"score"`
	FetchedAt string  `json:"fetched_at"`
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req queryReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	if req.Text == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	if s.Chroma == nil {
		w.WriteHeader(http.StatusBadGateway)
		writeJSON(w, map[string]any{"error": "chroma not configured"})
		return
	}

	// embed query using chroma
	vec, err := s.Chroma.Embed(r.Context(), req.Text)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		writeJSON(w, map[string]any{"error": "chroma embed failed: " + err.Error()})
		return
	}

	docs := s.Store.List()
	hits := make([]queryHit, 0, len(docs))
	for _, d := range docs {
		if d.Status != "ok" || len(d.Embedding) == 0 {
			continue
		}
		s := cosine(vec, d.Embedding)
		hits = append(hits, queryHit{Domain: d.Domain, Score: s, FetchedAt: d.FetchedAt.Format(time.RFC3339)})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > limit {
		hits = hits[:limit]
	}
	writeJSON(w, hits)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func parseBoolQuery(r *http.Request, key string) bool {
	v := strings.TrimSpace(r.URL.Query().Get(key))
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err == nil {
		return b
	}
	// accept common shorthands
	switch strings.ToLower(v) {
	case "1", "y", "yes", "on", "true":
		return true
	default:
		return false
	}
}
