package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type ChromaClient struct {
	BaseURL    string
	Collection string
	HTTP       *http.Client
}

type chromaUpsertReq struct {
	Collection string            `json:"collection"`
	ID         string            `json:"id"`
	Embedding  []float64         `json:"embedding"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type chromaDeleteReq struct {
	Collection string `json:"collection"`
	ID         string `json:"id"`
}

type chromaSimilarReq struct {
	Collection string `json:"collection"`
	Domain     string `json:"domain"`
	Limit      int    `json:"limit"`
}

type chromaSimilarResp struct {
	Domain string       `json:"domain"`
	Limit  int          `json:"limit"`
	Hits   []similarHit `json:"hits"`
	Hash   string       `json:"hash"`
}

func NewChromaClient(baseURL, collection string) *ChromaClient {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if collection == "" {
		collection = "publishers"
	}
	return &ChromaClient{
		BaseURL:    baseURL,
		Collection: collection,
		HTTP:       &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *ChromaClient) Upsert(ctx context.Context, id string, embedding []float64, metadata map[string]string) error {
	if c == nil || c.BaseURL == "" {
		return fmt.Errorf("chroma not configured")
	}
	id = strings.TrimSpace(strings.ToLower(id))
	if id == "" {
		return fmt.Errorf("id required")
	}
	if len(embedding) == 0 {
		return fmt.Errorf("embedding required")
	}
	req := chromaUpsertReq{Collection: c.Collection, ID: id, Embedding: embedding, Metadata: metadata}
	return c.post(ctx, "/v1/upsert", req, nil)
}

func (c *ChromaClient) Delete(ctx context.Context, id string) error {
	if c == nil || c.BaseURL == "" {
		return fmt.Errorf("chroma not configured")
	}
	id = strings.TrimSpace(strings.ToLower(id))
	if id == "" {
		return fmt.Errorf("id required")
	}
	req := chromaDeleteReq{Collection: c.Collection, ID: id}
	return c.post(ctx, "/v1/delete", req, nil)
}

func (c *ChromaClient) Similar(ctx context.Context, domain string, limit int) (chromaSimilarResp, error) {
	if c == nil || c.BaseURL == "" {
		return chromaSimilarResp{}, fmt.Errorf("chroma not configured")
	}
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return chromaSimilarResp{}, fmt.Errorf("domain required")
	}
	if limit <= 0 {
		limit = 10
	}
	req := chromaSimilarReq{Collection: c.Collection, Domain: domain, Limit: limit}
	var resp chromaSimilarResp
	if err := c.post(ctx, "/v1/similar", req, &resp); err != nil {
		return chromaSimilarResp{}, err
	}
	return resp, nil
}

func (c *ChromaClient) post(ctx context.Context, p string, reqBody any, out any) error {
	b, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}
	url := c.BaseURL + p
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("chroma http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return err
	}
	return nil
}
