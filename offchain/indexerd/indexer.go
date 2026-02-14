package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"time"

	"content-grid-chain/offchain/executor"

	"golang.org/x/net/html"
)

type Indexer struct {
	Cfg     Config
	Store   *Store
	Fetcher *executor.HTTPFetcher
	Embed   *executor.SentenceTransformerClient
	Chain   *ChainClient
	Chroma  *ChromaClient
}

func (i *Indexer) IndexOnce(ctx context.Context, domain string) PublisherDoc {
	domain = strings.TrimSpace(strings.ToLower(domain))
	doc := PublisherDoc{Domain: domain, FetchedAt: time.Now()}

	// Prefer llm.txt as embedding input when available, otherwise fall back to homepage.
	// We still try to fetch the homepage to extract Congrid badge info.
	var (
		embedRaw      []byte
		embedURL      string
		homeRaw       []byte
		homeURL       string
		lastErr       error
		usedLLMTxt    bool
		maxBody       = i.Cfg.MaxBodyBytes
		candidateLLM  = []string{fmt.Sprintf("https://%s/llm.txt", domain), fmt.Sprintf("http://%s/llm.txt", domain)}
		candidateHome = []string{fmt.Sprintf("https://%s/", domain), fmt.Sprintf("http://%s/", domain)}
	)

	for _, u := range candidateLLM {
		raw, err := i.Fetcher.Fetch(ctx, u)
		if err != nil {
			lastErr = err
			continue
		}
		embedRaw = raw
		embedURL = u
		usedLLMTxt = true
		break
	}

	for _, u := range candidateHome {
		raw, err := i.Fetcher.Fetch(ctx, u)
		if err != nil {
			lastErr = err
			continue
		}
		homeRaw = raw
		homeURL = u
		break
	}

	if embedRaw == nil {
		embedRaw = homeRaw
		embedURL = homeURL
		usedLLMTxt = false
	}
	if embedRaw == nil {
		doc.Status = "error"
		if lastErr != nil {
			doc.Error = lastErr.Error()
		}
		return doc
	}

	if int64(len(embedRaw)) > maxBody {
		embedRaw = embedRaw[:maxBody]
	}
	doc.SourceURL = embedURL
	doc.ResponseBytes = len(embedRaw)
	h := sha256.Sum256(embedRaw)
	doc.BodySHA256 = hex.EncodeToString(h[:])

	badgeRaw := homeRaw
	if badgeRaw == nil {
		badgeRaw = embedRaw
	}
	links, wallet := extractCongridBadgeInfo(string(badgeRaw), domain)
	doc.CongridLinks = links
	doc.Wallet = wallet

	if usedLLMTxt {
		// llm.txt is plain text; treat it as already normalized content.
		doc.Markdown = strings.TrimSpace(string(embedRaw))
	} else {
		processed, err := executor.PreprocessDocument(embedRaw)
		if err == nil {
			doc.Title = processed.Title
			doc.Markdown = processed.Markdown
		}
	}

	embedInput := []byte(doc.Markdown)
	if len(embedInput) == 0 {
		embedInput = embedRaw
	}
	vec, err := i.Embed.Embed(ctx, embedInput)
	if err != nil {
		doc.Status = "error"
		doc.Error = err.Error()
		return doc
	}
	doc.EmbeddingDim = len(vec)
	doc.EmbeddingNormalized = i.Cfg.NormalizeEmbeddings

	// Persist embedding to Chroma (if configured) for scalable similarity search.
	if i.Chroma != nil && i.Cfg.ChromaBaseURL != "" {
		meta := map[string]string{"source_url": doc.SourceURL}
		_ = i.Chroma.Upsert(ctx, doc.Domain, vec, meta)
		// Avoid keeping large embeddings in memory by default when Chroma is enabled.
		doc.Embedding = nil
		doc.EmbeddingExternal = true
	} else {
		doc.Embedding = vec
		doc.EmbeddingExternal = false
	}

	sigHex, algo := signatureHex(vec, i.Cfg.SignatureBits)
	doc.Signature = sigHex
	doc.SignatureBits = i.Cfg.SignatureBits
	doc.SignatureAlgo = algo

	doc.Status = "ok"
	doc.Error = ""
	return doc
}

func (i *Indexer) IndexAll(ctx context.Context) {
	domains, err := i.publisherDomains(ctx)
	if err != nil {
		log.Printf("failed to resolve publishers: %v", err)
		return
	}

	keep := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		keep[d] = struct{}{}
	}
	for _, existing := range i.Store.List() {
		d := strings.TrimSpace(strings.ToLower(existing.Domain))
		if d == "" {
			continue
		}
		if _, ok := keep[d]; ok {
			continue
		}
		i.Store.Delete(d)
		if i.Chroma != nil && i.Cfg.ChromaBaseURL != "" {
			ctxDel, cancel := context.WithTimeout(ctx, 5*time.Second)
			if err := i.Chroma.Delete(ctxDel, d); err != nil {
				log.Printf("prune chroma %s failed: %v", d, err)
			}
			cancel()
		}
		log.Printf("pruned inactive publisher %s from index", d)
	}

	for _, d := range domains {
		ctx2, cancel := context.WithTimeout(ctx, i.Cfg.FetchTimeout()+10*time.Second)
		res := i.IndexOnce(ctx2, d)
		cancel()
		i.Store.Put(res)
		log.Printf("indexed %s status=%s congrid_links=%d dim=%d sig_bits=%d", d, res.Status, res.CongridLinks, res.EmbeddingDim, res.SignatureBits)
	}
}

func (i *Indexer) publisherDomains(ctx context.Context) ([]string, error) {
	seen := make(map[string]struct{})
	out := make([]string, 0)

	add := func(d string) {
		d = strings.TrimSpace(strings.ToLower(d))
		if d == "" {
			return
		}
		if _, ok := seen[d]; ok {
			return
		}
		seen[d] = struct{}{}
		out = append(out, d)
	}

	for _, d := range i.Cfg.Publishers {
		norm := strings.TrimSpace(strings.ToLower(d))
		if norm == "" {
			continue
		}
		if i.Chain != nil {
			ctx2, cancel := context.WithTimeout(ctx, i.Cfg.ChainTimeout())
			active, err := i.Chain.IsPublisherActive(ctx2, norm)
			cancel()
			if err != nil {
				return nil, err
			}
			if !active {
				continue
			}
		}
		add(norm)
	}

	if i.Chain != nil {
		ctx2, cancel := context.WithTimeout(ctx, i.Cfg.ChainTimeout())
		domains, err := i.Chain.ListPublishers(ctx2, i.Cfg.ChainPageLimit)
		cancel()
		if err != nil {
			return nil, err
		}
		for _, d := range domains {
			add(d)
		}
	}

	sort.Strings(out)
	return out, nil
}

func extractCongridBadgeInfo(pageHTML, expectedDomain string) (count int, wallet string) {
	expectedDomain = strings.TrimSpace(strings.ToLower(expectedDomain))
	if expectedDomain == "" {
		return 0, ""
	}
	// allow expected domain with port but publisher param without
	expectedNoPort := expectedDomain
	if idx := strings.LastIndex(expectedNoPort, ":"); idx != -1 {
		expectedNoPort = expectedNoPort[:idx]
	}

	tok := html.NewTokenizer(strings.NewReader(pageHTML))
	inAnchor := false
	for {
		switch tok.Next() {
		case html.ErrorToken:
			return count, wallet
		case html.StartTagToken, html.SelfClosingTagToken:
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
				inAnchor = isOfficialCongridHref(href)
				continue
			}
			if inAnchor && tag == "img" {
				src := ""
				for hasAttr {
					k, v, more := tok.TagAttr()
					hasAttr = more
					if strings.EqualFold(string(k), "src") {
						src = string(v)
					}
				}
				pub, w := parseBadgeSrc(src)
				if pub == expectedDomain || pub == expectedNoPort {
					count++
					if wallet == "" && w != "" {
						wallet = w
					}
				}
			}
		case html.EndTagToken:
			name, _ := tok.TagName()
			if strings.EqualFold(string(name), "a") {
				inAnchor = false
			}
		}
	}
}

func isOfficialCongridHref(raw string) bool {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if u.Scheme != "https" {
		return false
	}
	h := strings.ToLower(u.Host)
	if h != "congrid.net" && h != "www.congrid.net" {
		return false
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	if u.Path != "" && u.Path != "/" {
		return false
	}
	return true
}

func parseBadgeSrc(raw string) (publisher string, wallet string) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	if u.Scheme != "https" {
		return "", ""
	}
	h := strings.ToLower(u.Host)
	if h != "congrid.net" && h != "www.congrid.net" {
		return "", ""
	}
	q := u.Query()
	publisher = strings.TrimSpace(strings.ToLower(q.Get("publisher")))
	wallet = strings.TrimSpace(q.Get("wallet"))
	return publisher, wallet
}
