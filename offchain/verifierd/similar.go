package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	similarTopN = 15
)

type indexerdSimilarReq struct {
	Domain string `json:"domain"`
	Limit  int    `json:"limit"`
}

type indexerdSimilarResp struct {
	Domain string `json:"domain"`
	Limit  int    `json:"limit"`
	Hits   []struct {
		Domain string  `json:"domain"`
		Score  float64 `json:"score"`
	} `json:"hits"`
	Hash string `json:"hash"`
}

func fetchExpectedSimilar(ctx context.Context, baseURL, domain string) (domains []string, setHash string, err error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, "", fmt.Errorf("indexerd_base_url not set")
	}
	payload, _ := json.Marshal(indexerdSimilarReq{Domain: domain, Limit: similarTopN})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/v1/similar", bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("content-type", "application/json")
	cli := &http.Client{Timeout: 10 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("indexerd /v1/similar http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out indexerdSimilarResp
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, "", err
	}
	doms := make([]string, 0, len(out.Hits))
	for _, h := range out.Hits {
		d := normalizeDomainOnly(h.Domain)
		if d != "" {
			doms = append(doms, d)
		}
	}
	// Ensure stable set hash input by sorting.
	sort.Strings(doms)
	setHash = strings.TrimSpace(out.Hash)
	if setHash == "" {
		sum := sha256.Sum256([]byte(strings.Join(doms, "\n")))
		setHash = hex.EncodeToString(sum[:])
	}
	return doms, setHash, nil
}

func parseObservedSimilarDomains(pageHTML string) ([]string, error) {
	// Expect: <div id="congrid-similar"> ... <a ... href="https://example.com/..."> ...
	// But per spec we only verify the domain (no paths).
	tok := html.NewTokenizer(strings.NewReader(pageHTML))

	inContainer := false
	containerDivDepth := 0
	seen := map[string]struct{}{}
	out := []string{}

	for {
		tt := tok.Next()
		switch tt {
		case html.ErrorToken:
			return out, nil
		case html.StartTagToken:
			name, hasAttr := tok.TagName()
			tag := strings.ToLower(string(name))
			if tag == "div" {
				id := ""
				for hasAttr {
					k, v, more := tok.TagAttr()
					hasAttr = more
					if strings.EqualFold(string(k), "id") {
						id = string(v)
					}
				}
				if id == "congrid-similar" {
					inContainer = true
					containerDivDepth = 1
					continue
				}
				if inContainer {
					containerDivDepth++
				}
				continue
			}

			if inContainer {
				if tag == "a" {
					href := ""
					for hasAttr {
						k, v, more := tok.TagAttr()
						hasAttr = more
						if strings.EqualFold(string(k), "href") {
							href = string(v)
						}
					}
					d := normalizeDomainOnly(href)
					if d != "" {
						if _, ok := seen[d]; !ok {
							seen[d] = struct{}{}
							out = append(out, d)
						}
					}
				}
			}

		case html.EndTagToken:
			name, _ := tok.TagName()
			if inContainer && strings.EqualFold(string(name), "div") {
				containerDivDepth--
				if containerDivDepth <= 0 {
					inContainer = false
					containerDivDepth = 0
				}
			}
		}
	}
}

func normalizeDomainOnly(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Allow bare domains without scheme.
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return ""
	}
	return host
}

func sha256HexOfSet(domains []string) string {
	clean := make([]string, 0, len(domains))
	seen := map[string]struct{}{}
	for _, d := range domains {
		d = normalizeDomainOnly(d)
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		clean = append(clean, d)
	}
	sort.Strings(clean)
	sum := sha256.Sum256([]byte(strings.Join(clean, "\n")))
	return hex.EncodeToString(sum[:])
}

func overlapCount(a, b []string) int {
	set := map[string]struct{}{}
	for _, x := range a {
		set[normalizeDomainOnly(x)] = struct{}{}
	}
	count := 0
	for _, y := range b {
		if _, ok := set[normalizeDomainOnly(y)]; ok {
			count++
		}
	}
	return count
}
