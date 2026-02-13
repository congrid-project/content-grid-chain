package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

type similarReq struct {
	Domain string `json:"domain"`
	Limit  int    `json:"limit"`
}

type similarResp struct {
	Domain string       `json:"domain"`
	Limit  int          `json:"limit"`
	Hits   []similarHit `json:"hits"`
	Hash   string       `json:"hash"`
}

type similarHit struct {
	Domain string  `json:"domain"`
	Score  float64 `json:"score"`
}

// handleSimilar returns the top-N similar publisher domains using stored embeddings.
// This is intended as a simple, deterministic reference implementation for verifier operators.
func (s *Server) handleSimilar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req similarReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	req.Domain = strings.TrimSpace(strings.ToLower(req.Domain))
	if req.Domain == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	// If chroma is configured, delegate similarity search to it (persistent + scalable).
	if s.Chroma != nil && s.Cfg.ChromaBaseURL != "" {
		resp, err := s.Chroma.Similar(r.Context(), req.Domain, limit)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			writeJSON(w, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, resp)
		return
	}

	// Fallback: in-memory cosine scan (dev only).
	target, ok := s.Store.Get(req.Domain)
	if !ok || target.Status != "ok" || len(target.Embedding) == 0 {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]any{"error": "domain not indexed"})
		return
	}

	docs := s.Store.List()
	hits := make([]similarHit, 0, len(docs))
	for _, d := range docs {
		if d.Domain == req.Domain {
			continue
		}
		if d.Status != "ok" || len(d.Embedding) == 0 {
			continue
		}
		score := cosine(target.Embedding, d.Embedding)
		hits = append(hits, similarHit{Domain: d.Domain, Score: score})
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > limit {
		hits = hits[:limit]
	}

	// Hash the expected set (domain-only) for cheap on-chain attestation.
	domains := make([]string, 0, len(hits))
	for _, h := range hits {
		d := strings.TrimSpace(strings.ToLower(h.Domain))
		if d != "" {
			domains = append(domains, d)
		}
	}
	sort.Strings(domains)
	setHash := sha256.Sum256([]byte(strings.Join(domains, "\n")))

	writeJSON(w, similarResp{
		Domain: req.Domain,
		Limit:  limit,
		Hits:   hits,
		Hash:   hex.EncodeToString(setHash[:]),
	})
}
