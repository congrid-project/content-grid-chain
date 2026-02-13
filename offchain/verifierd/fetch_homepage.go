package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func fetchHomepageHTML(ctx context.Context, scheme, domain string) (string, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return "", fmt.Errorf("domain required")
	}
	scheme = strings.TrimSpace(strings.ToLower(scheme))
	candidates := []string{}
	if scheme == "http" || scheme == "https" {
		candidates = append(candidates, fmt.Sprintf("%s://%s/", scheme, domain))
	} else {
		candidates = append(candidates, fmt.Sprintf("https://%s/", domain), fmt.Sprintf("http://%s/", domain))
	}

	cli := &http.Client{Timeout: 10 * time.Second}
	var lastErr error
	for _, u := range candidates {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			lastErr = err
			continue
		}
		resp, err := cli.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 400 {
			lastErr = fmt.Errorf("http %d", resp.StatusCode)
			continue
		}
		if len(b) == 0 {
			lastErr = fmt.Errorf("empty body")
			continue
		}
		return string(b), nil
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("failed to fetch homepage")
}
