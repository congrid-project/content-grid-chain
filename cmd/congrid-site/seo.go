package main

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func seoCanonicalURL(baseURL, path, lang string) string {
	if baseURL == "" {
		return ""
	}
	u, err := url.Parse(strings.TrimRight(baseURL, "/") + path)
	if err != nil {
		return ""
	}
	q := url.Values{}
	if lang != "en" && validLanguage(lang) {
		q.Set("lang", lang)
	}
	u.RawQuery = q.Encode()
	u.Fragment = ""
	return u.String()
}

func seoLanguageURL(canonical, lang string) string {
	u, err := url.Parse(canonical)
	if err != nil {
		return ""
	}
	q := u.Query()
	q.Del("lang")
	if lang != "en" {
		q.Set("lang", lang)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

type sitemapEntry struct {
	Location string `xml:"loc"`
	Modified string `xml:"lastmod,omitempty"`
}
type siteMap struct {
	XMLName   xml.Name       `xml:"urlset"`
	Namespace string         `xml:"xmlns,attr"`
	Entries   []sitemapEntry `xml:"url"`
}

func (c *cmsApp) handleSitemap(w http.ResponseWriter, r *http.Request) {
	posts, _, err := c.store.listPosts(r.Context(), cmsPostFilter{Public: true})
	if err != nil {
		c.internalError(w, err)
		return
	}
	sitemap := siteMap{Namespace: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, path := range []string{"/", "/publishers", "/verifiers", "/docs", "/blog"} {
		for _, lang := range []string{"en", "zh", "fr"} {
			sitemap.Entries = append(sitemap.Entries, sitemapEntry{Location: seoCanonicalURL(c.baseURL, path, lang)})
		}
	}
	for _, post := range posts {
		sitemap.Entries = append(sitemap.Entries, sitemapEntry{Location: c.baseURL + post.URL(), Modified: seoTimestamp(post.UpdatedAt)})
	}
	c.writeXML(w, sitemap)
}

func (c *cmsApp) handleRobots(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "User-agent: *\nDisallow: /cms\n\nSitemap: %s/sitemap.xml\n", c.baseURL)
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	Category    string `xml:"category"`
	Published   string `xml:"pubDate"`
}
type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}
type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Channel rssChannel `xml:"channel"`
}

func (c *cmsApp) handleFeed(w http.ResponseWriter, r *http.Request) {
	posts, _, err := c.store.listPosts(r.Context(), cmsPostFilter{Public: true, Limit: 50})
	if err != nil {
		c.internalError(w, err)
		return
	}
	feed := rssFeed{Version: "2.0", Channel: rssChannel{Title: "Congrid Blog", Link: c.baseURL + "/blog", Description: "News, guides and protocol updates from Congrid."}}
	for _, post := range posts {
		feed.Channel.Items = append(feed.Channel.Items, rssItem{Title: post.Title, Link: c.baseURL + post.URL(), GUID: c.baseURL + post.URL(), Description: post.Summary, Category: post.Category, Published: time.Unix(post.PublishedAt, 0).UTC().Format(time.RFC1123Z)})
	}
	c.writeXML(w, feed)
}

func (c *cmsApp) writeXML(w http.ResponseWriter, value any) {
	data, err := xml.MarshalIndent(value, "", "  ")
	if err != nil {
		c.internalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(xml.Header))
	_, _ = w.Write(data)
}
