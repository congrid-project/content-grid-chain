package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPFetcher fetches remote documents with sane defaults for the indexer.
type HTTPFetcher struct {
	client *http.Client
}

// NewHTTPFetcher builds a fetcher using the provided timeout.
func NewHTTPFetcher(timeout time.Duration) *HTTPFetcher {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HTTPFetcher{client: &http.Client{Timeout: timeout}}
}

// Fetch retrieves the document at the given URL.
func (f *HTTPFetcher) Fetch(ctx context.Context, endpoint string) ([]byte, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("endpoint required")
	}
	client := f.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", "congrid-indexer/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("fetch %s: status %d", endpoint, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", endpoint, err)
	}
	return body, nil
}
