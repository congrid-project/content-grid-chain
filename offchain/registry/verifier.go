package registryoffchain

import (
	"context"
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
	// DefaultVerifyPath is the homepage path used for publisher ownership verification.
	DefaultVerifyPath = "/"
)

// HTTPContentVerifier fetches a page from a domain and ensures it contains Congrid links.
type HTTPContentVerifier struct {
	Client *http.Client
	Scheme string
}

type LeaseExpectation struct {
	SlotID    string
	LeaseID   string
	TargetURL string
}

// Verify ensures the homepage includes a Congrid verification anchor bound to the owner.
//
// Required structure:
//   - An anchor: <a href="https://congrid.net"> (or https://www.congrid.net/) with NO query/fragment.
//   - The anchor MUST wrap an <img ...>.
//   - The image src URL MUST include publisher + wallet information (either in query or path),
//     so it can be used for future attribution/statistics.
//
// Binding rules:
//   - The image must encode publisher=<domain> (port optional) and wallet=<owner> (bech32).
func (v HTTPContentVerifier) Verify(ctx context.Context, domain, owner string) error {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return fmt.Errorf("owner required")
	}
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return fmt.Errorf("domain required")
	}

	client := v.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	schemes := []string{strings.TrimSpace(v.Scheme)}
	if schemes[0] == "" {
		// Default to https, but fall back to http for local/dev setups.
		schemes = []string{"https", "http"}
	}

	var lastErr error
	for _, scheme := range schemes {
		endpoint, err := BuildPageURL(scheme, domain, DefaultVerifyPath)
		if err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to reach %s: %w", endpoint, err)
			continue
		}

		func() {
			defer resp.Body.Close()
			if resp.StatusCode >= 400 {
				lastErr = fmt.Errorf("verification endpoint %s returned status %d", endpoint, resp.StatusCode)
				return
			}
			body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			if err != nil {
				lastErr = fmt.Errorf("failed to read verification body: %w", err)
				return
			}
			if !hasCongridVerificationAnchor(string(body), domain, owner) {
				lastErr = fmt.Errorf("no congrid verification anchor found for owner %s at %s", owner, endpoint)
				return
			}
			lastErr = nil
		}()

		if lastErr == nil {
			return nil
		}
	}

	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("verification failed")
}

// VerifyWithLeases validates the publisher anchor and required lease anchors.
func (v HTTPContentVerifier) VerifyWithLeases(ctx context.Context, domain, owner string, leases []LeaseExpectation) error {
	if err := v.Verify(ctx, domain, owner); err != nil {
		return err
	}
	if len(leases) == 0 {
		return nil
	}

	page, endpoint, err := v.fetchHomepageHTML(ctx, domain)
	if err != nil {
		return err
	}
	if err := ensureLeaseAnchors(page, leases); err != nil {
		return fmt.Errorf("lease verification failed at %s: %w", endpoint, err)
	}
	return nil
}

func (v HTTPContentVerifier) fetchHomepageHTML(ctx context.Context, domain string) (string, string, error) {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return "", "", fmt.Errorf("domain required")
	}

	client := v.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	schemes := []string{strings.TrimSpace(v.Scheme)}
	if schemes[0] == "" {
		schemes = []string{"https", "http"}
	}

	var lastErr error
	var lastEndpoint string
	for _, scheme := range schemes {
		endpoint, err := BuildPageURL(scheme, domain, DefaultVerifyPath)
		if err != nil {
			return "", "", err
		}
		lastEndpoint = endpoint

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return "", endpoint, err
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to reach %s: %w", endpoint, err)
			continue
		}
		body, err := func() ([]byte, error) {
			defer resp.Body.Close()
			if resp.StatusCode >= 400 {
				return nil, fmt.Errorf("verification endpoint %s returned status %d", endpoint, resp.StatusCode)
			}
			return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		}()
		if err != nil {
			lastErr = err
			continue
		}
		if len(body) == 0 {
			lastErr = fmt.Errorf("empty body")
			continue
		}
		return string(body), endpoint, nil
	}

	if lastErr != nil {
		return "", lastEndpoint, lastErr
	}
	return "", lastEndpoint, fmt.Errorf("failed to fetch homepage")
}

func hasCongridVerificationAnchor(pageHTML, expectedDomain, expectedOwner string) bool {
	tok := html.NewTokenizer(strings.NewReader(pageHTML))

	// Track whether we're inside a candidate <a href="https://congrid.net"> ... </a>
	inCandidateAnchor := false
	anchorDepth := 0

	for {
		tt := tok.Next()
		switch tt {
		case html.ErrorToken:
			return false
		case html.StartTagToken:
			name, hasAttr := tok.TagName()
			tag := strings.ToLower(string(name))

			if tag == "a" {
				href := ""
				for hasAttr {
					k, v, more := tok.TagAttr()
					hasAttr = more
					if strings.EqualFold(string(k), "href") {
						href = string(v)
					}
				}

				if isOfficialCongridAnchorHref(href) {
					inCandidateAnchor = true
					anchorDepth = 1
				} else if inCandidateAnchor {
					anchorDepth++
				}
				continue
			}

			if inCandidateAnchor {
				anchorDepth++
				if tag == "img" {
					src := ""
					for hasAttr {
						k, v, more := tok.TagAttr()
						hasAttr = more
						if strings.EqualFold(string(k), "src") {
							src = string(v)
						}
					}
					pub, wallet, ok := extractPublisherInfoFromBadgeImageSrc(src)
					if ok && wallet == expectedOwner && publisherMatches(pub, expectedDomain) {
						return true
					}
				}
			}

		case html.SelfClosingTagToken:
			name, hasAttr := tok.TagName()
			tag := strings.ToLower(string(name))
			if inCandidateAnchor && tag == "img" {
				src := ""
				for hasAttr {
					k, v, more := tok.TagAttr()
					hasAttr = more
					if strings.EqualFold(string(k), "src") {
						src = string(v)
					}
				}
				pub, wallet, ok := extractPublisherInfoFromBadgeImageSrc(src)
				if ok && wallet == expectedOwner && publisherMatches(pub, expectedDomain) {
					return true
				}
			}

		case html.EndTagToken:
			name, _ := tok.TagName()
			tag := strings.ToLower(string(name))
			if inCandidateAnchor {
				anchorDepth--
				if tag == "a" || anchorDepth <= 0 {
					inCandidateAnchor = false
					anchorDepth = 0
				}
			}
		}
	}
}

func isOfficialCongridAnchorHref(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Host)
	if host != "congrid.net" && host != "www.congrid.net" {
		return false
	}
	// No query/fragment on the official link.
	if u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	// Only allow empty path or '/'
	if u.Path != "" && u.Path != "/" {
		return false
	}
	return true
}

func extractPublisherInfoFromBadgeImageSrc(raw string) (publisher string, wallet string, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}
	// Require the badge image to be served from congrid.net.
	if u.Scheme != "https" {
		return "", "", false
	}
	host := strings.ToLower(u.Host)
	if host != "congrid.net" && host != "www.congrid.net" {
		return "", "", false
	}

	q := u.Query()
	publisher = strings.TrimSpace(strings.ToLower(q.Get("publisher")))
	wallet = strings.TrimSpace(q.Get("wallet"))
	if publisher != "" && wallet != "" {
		return publisher, wallet, true
	}

	// Optional best-effort path extraction (if we ever move publisher/wallet into the URL path).
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	for _, s := range segs {
		sl := strings.ToLower(strings.TrimSpace(s))
		if publisher == "" && strings.Contains(sl, ".") {
			publisher = sl
		}
		if wallet == "" && strings.HasPrefix(strings.TrimSpace(s), "grid1") {
			wallet = strings.TrimSpace(s)
		}
	}
	if publisher != "" && wallet != "" {
		return publisher, wallet, true
	}
	return publisher, wallet, false
}

func publisherMatches(found, expected string) bool {
	found = strings.TrimSpace(strings.ToLower(found))
	expected = strings.TrimSpace(strings.ToLower(expected))
	if found == "" || expected == "" {
		return false
	}
	if found == expected {
		return true
	}
	// Allow expected with port but found without.
	if idx := strings.LastIndex(expected, ":"); idx != -1 {
		if expected[:idx] == found {
			return true
		}
	}
	// Allow found with port but expected without.
	if idx := strings.LastIndex(found, ":"); idx != -1 {
		if found[:idx] == expected {
			return true
		}
	}
	return false
}

// BuildPageURL constructs the URL hosting the homepage content.
func BuildPageURL(scheme, domain, path string) (string, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return "", fmt.Errorf("domain required")
	}
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		cleanPath = DefaultVerifyPath
	}
	if !strings.HasPrefix(cleanPath, "/") {
		cleanPath = "/" + cleanPath
	}
	if scheme == "" {
		scheme = "https"
	}
	u := &url.URL{Scheme: scheme, Host: domain, Path: cleanPath}
	return u.String(), nil
}

func ensureLeaseAnchors(pageHTML string, leases []LeaseExpectation) error {
	if len(leases) == 0 {
		return nil
	}
	expected := map[string]LeaseExpectation{}
	for _, lease := range leases {
		slotID := strings.TrimSpace(lease.SlotID)
		leaseID := strings.TrimSpace(lease.LeaseID)
		target := strings.TrimSpace(lease.TargetURL)
		if slotID == "" || leaseID == "" || target == "" {
			continue
		}
		key := slotID + ":" + leaseID
		expected[key] = LeaseExpectation{SlotID: slotID, LeaseID: leaseID, TargetURL: target}
	}
	if len(expected) == 0 {
		return nil
	}
	found := map[string]bool{}

	tok := html.NewTokenizer(strings.NewReader(pageHTML))
	for {
		tt := tok.Next()
		switch tt {
		case html.ErrorToken:
			if len(found) == len(expected) {
				return nil
			}
			missing := missingLeaseKeys(expected, found)
			return fmt.Errorf("missing lease anchors: %s", strings.Join(missing, ", "))
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := tok.TagName()
			tag := strings.ToLower(string(name))
			if tag != "a" {
				continue
			}
			var href, slotID, leaseID string
			for hasAttr {
				k, v, more := tok.TagAttr()
				hasAttr = more
				key := strings.ToLower(string(k))
				val := strings.TrimSpace(string(v))
				switch key {
				case "href":
					href = val
				case "data-congrid-slot":
					slotID = val
				case "data-congrid-lease":
					leaseID = val
				}
			}
			if slotID == "" || leaseID == "" {
				continue
			}
			key := slotID + ":" + leaseID
			exp, ok := expected[key]
			if !ok {
				continue
			}
			if leaseHrefMatches(exp.TargetURL, href) {
				found[key] = true
				if len(found) == len(expected) {
					return nil
				}
			}
		}
	}
}

func missingLeaseKeys(expected map[string]LeaseExpectation, found map[string]bool) []string {
	missing := make([]string, 0)
	for key := range expected {
		if !found[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	return missing
}

func leaseHrefMatches(expected, href string) bool {
	expected = strings.TrimSpace(expected)
	href = strings.TrimSpace(href)
	if expected == "" || href == "" {
		return false
	}

	expURL, err := url.Parse(expected)
	if err != nil {
		return false
	}
	if expURL.Host == "" {
		return false
	}
	if strings.HasPrefix(href, "//") {
		href = "https:" + href
	}
	hrefURL, err := url.Parse(href)
	if err != nil {
		return false
	}
	if hrefURL.Host == "" {
		return false
	}
	if !strings.EqualFold(expURL.Host, hrefURL.Host) {
		return false
	}
	return normalizeLinkPath(expURL.Path) == normalizeLinkPath(hrefURL.Path)
}

func normalizeLinkPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if path != "/" {
		path = strings.TrimRight(path, "/")
		if path == "" {
			return "/"
		}
	}
	return path
}
