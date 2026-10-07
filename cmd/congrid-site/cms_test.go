package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

const cmsTestPassword = "test-cms-password-12345"

func testCMS(t *testing.T) (*cmsApp, *http.ServeMux) {
	t.Helper()
	templates, err := buildPageTemplates(siteFS)
	require.NoError(t, err)
	store, err := openCMSStore(context.Background(), filepath.Join(t.TempDir(), "cms.db"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	require.NoError(t, store.configureAdmin(context.Background(), "editor", cmsTestPassword, false))
	c, err := newCMSApp(&server{templates: templates}, store, "https://congrid.net")
	require.NoError(t, err)
	mux := http.NewServeMux()
	c.registerRoutes(mux)
	mux.HandleFunc("GET /{$}", c.srv.handleHome(c.baseURL))
	return c, mux
}

func cmsTestRequest(handler http.Handler, method, path string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	if method == http.MethodPost {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

func cmsCookie(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("missing cookie %s", name)
	return nil
}

func cmsTestLogin(t *testing.T, mux http.Handler) (*http.Cookie, string) {
	t.Helper()
	w := cmsTestRequest(mux, "GET", "/cms/login", nil)
	require.Equal(t, http.StatusOK, w.Code)
	csrfCookie := cmsCookie(t, w, cmsLoginCookie)
	w = cmsTestRequest(mux, "POST", "/cms/login", url.Values{"csrf": {csrfCookie.Value}, "username": {"editor"}, "password": {cmsTestPassword}}, csrfCookie)
	require.Equal(t, http.StatusSeeOther, w.Code, w.Body.String())
	cookie := cmsCookie(t, w, cmsSessionCookie)
	w = cmsTestRequest(mux, "GET", "/cms/posts/new", nil, cookie)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	match := regexp.MustCompile(`name="csrf" value="([^"]+)"`).FindStringSubmatch(w.Body.String())
	require.Len(t, match, 2)
	return cookie, match[1]
}

func cmsTestPost() cmsPost {
	return cmsPost{Slug: "network-update", Title: "Congrid network update", Summary: "An update for publishers.", Body: "## What changed\n\n**Useful content** and [a guide](/docs).", Category: "News", Language: "en", Author: "Congrid", Status: "draft"}
}

func cmsTestPostForm(p cmsPost, csrf string) url.Values {
	return url.Values{"csrf": {csrf}, "slug": {p.Slug}, "title": {p.Title}, "summary": {p.Summary}, "body": {p.Body}, "category": {p.Category}, "language": {p.Language}, "author": {p.Author}, "cover_url": {p.CoverURL}, "seo_title": {p.SEOTitle}, "seo_description": {p.SEODescription}, "status": {p.Status}, "revision": {strconv.FormatInt(p.Revision, 10)}}
}

func TestCMSPublicationLifecycleAndSEO(t *testing.T) {
	c, mux := testCMS(t)
	cookie, csrf := cmsTestLogin(t, mux)
	p := cmsTestPost()
	w := cmsTestRequest(mux, "POST", "/cms/posts/new", cmsTestPostForm(p, csrf), cookie)
	require.Equal(t, http.StatusSeeOther, w.Code, w.Body.String())
	p, err := c.store.postByID(context.Background(), 1)
	require.NoError(t, err)
	for _, path := range []string{"/blog", "/sitemap.xml", "/feed.xml"} {
		w = cmsTestRequest(mux, "GET", path, nil)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), p.Title)
		require.NotContains(t, w.Body.String(), p.URL())
	}
	require.Equal(t, http.StatusNotFound, cmsTestRequest(mux, "GET", p.URL(), nil, cookie).Code)
	require.Equal(t, http.StatusSeeOther, cmsTestRequest(mux, "GET", "/cms/posts/1/preview", nil).Code)
	w = cmsTestRequest(mux, "GET", "/cms/posts/1/preview", nil, cookie)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "noindex, nofollow", w.Header().Get("X-Robots-Tag"))
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	require.Contains(t, w.Body.String(), "<strong>Useful content</strong>")
	require.NotContains(t, w.Body.String(), "application/ld+json")
	p.Status = "published"
	p.Language = "zh"
	p.Title = "Congrid 更新 <script>alert(1)</script>"
	p.SEOTitle = "Congrid 发布动态"
	p.SEODescription = "网络公告与发布者指南。"
	p.CoverURL = "/static/assets/congrid-architecture.png"
	w = cmsTestRequest(mux, "POST", "/cms/posts/1/edit", cmsTestPostForm(p, csrf), cookie)
	require.Equal(t, http.StatusSeeOther, w.Code, w.Body.String())
	p, err = c.store.postByID(context.Background(), 1)
	require.NoError(t, err)
	require.Positive(t, p.PublishedAt)
	w = cmsTestRequest(mux, "GET", p.URL()+"?lang=fr", nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, "zh", w.Header().Get("Content-Language"))
	body := w.Body.String()
	require.Contains(t, body, `<html lang="zh">`)
	require.Contains(t, body, `<title>Congrid 发布动态</title>`)
	require.Contains(t, body, `<link rel="canonical" href="https://congrid.net/blog/network-update"`)
	require.NotContains(t, body, `hreflang="fr" href=`)
	require.Contains(t, body, `property="og:image" content="https://congrid.net/static/assets/congrid-architecture.png"`)
	require.NotContains(t, body, "<script>alert(1)</script>")
	match := regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`).FindStringSubmatch(body)
	require.Len(t, match, 2)
	var structured map[string]any
	require.NoError(t, json.Unmarshal([]byte(match[1]), &structured))
	require.Equal(t, p.Title, structured["headline"])
	require.Equal(t, "BlogPosting", structured["@type"])
	require.Equal(t, "zh", structured["inLanguage"])
	w = cmsTestRequest(mux, "GET", "/sitemap.xml", nil)
	var sitemap siteMap
	require.NoError(t, xml.Unmarshal(w.Body.Bytes(), &sitemap))
	require.Contains(t, w.Body.String(), "https://congrid.net"+p.URL())
	require.NotContains(t, w.Body.String(), "/cms")
	w = cmsTestRequest(mux, "GET", "/feed.xml", nil)
	var feed rssFeed
	require.NoError(t, xml.Unmarshal(w.Body.Bytes(), &feed))
	require.Len(t, feed.Channel.Items, 1)
	require.Equal(t, p.Title, feed.Channel.Items[0].Title)
	require.Equal(t, p.Summary, feed.Channel.Items[0].Description)
	// Updating cannot silently overwrite another editor's revision.
	p.Revision--
	w = cmsTestRequest(mux, "POST", "/cms/posts/1/edit", cmsTestPostForm(p, csrf), cookie)
	require.Equal(t, http.StatusConflict, w.Code)
	p.Revision++
	p.Status = "draft"
	w = cmsTestRequest(mux, "POST", "/cms/posts/1/edit", cmsTestPostForm(p, csrf), cookie)
	require.Equal(t, http.StatusSeeOther, w.Code)
	require.Equal(t, http.StatusNotFound, cmsTestRequest(mux, "GET", p.URL(), nil).Code)
	require.NotContains(t, cmsTestRequest(mux, "GET", "/feed.xml", nil).Body.String(), p.URL())
	require.NotContains(t, cmsTestRequest(mux, "GET", "/sitemap.xml", nil).Body.String(), p.URL())
	p, err = c.store.postByID(context.Background(), 1)
	require.NoError(t, err)
	w = cmsTestRequest(mux, "POST", "/cms/posts/1/delete", url.Values{"csrf": {csrf}, "revision": {strconv.FormatInt(p.Revision, 10)}}, cookie)
	require.Equal(t, http.StatusBadRequest, w.Code)
	w = cmsTestRequest(mux, "POST", "/cms/posts/1/delete", url.Values{"csrf": {csrf}, "revision": {strconv.FormatInt(p.Revision, 10)}, "confirm": {"delete"}}, cookie)
	require.Equal(t, http.StatusSeeOther, w.Code)
	_, err = c.store.postByID(context.Background(), 1)
	require.ErrorIs(t, err, sql.ErrNoRows)
}

func TestCMSAuthenticationAndIsolation(t *testing.T) {
	c, mux := testCMS(t)
	for _, path := range []string{"/cms", "/cms/posts", "/cms/posts/new", "/cms/posts/1/edit", "/cms/posts/1/preview"} {
		w := cmsTestRequest(mux, "GET", path, nil)
		require.Equal(t, http.StatusSeeOther, w.Code, path)
		require.Equal(t, "/cms/login", w.Header().Get("Location"))
	}
	for _, path := range []string{"/cms/posts/new", "/cms/posts/1/edit", "/cms/posts/1/delete", "/cms/logout"} {
		require.Equal(t, http.StatusUnauthorized, cmsTestRequest(mux, "POST", path, url.Values{}).Code, path)
	}
	cookie, csrf := cmsTestLogin(t, mux)
	require.True(t, cookie.HttpOnly)
	require.True(t, cookie.Secure)
	require.Equal(t, "/cms", cookie.Path)
	require.Equal(t, http.SameSiteStrictMode, cookie.SameSite)
	var storedHash string
	require.NoError(t, c.store.db.QueryRow(`SELECT token_hash FROM cms_sessions`).Scan(&storedHash))
	require.NotEqual(t, cookie.Value, storedHash)
	require.Equal(t, cmsTokenHash(cookie.Value), storedHash)
	require.Equal(t, http.StatusForbidden, cmsTestRequest(mux, "POST", "/cms/posts/new", cmsTestPostForm(cmsTestPost(), "wrong"), cookie).Code)
	r := httptest.NewRequest("POST", "/cms/posts/new", strings.NewReader(cmsTestPostForm(cmsTestPost(), csrf).Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "https://other.example")
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	require.Equal(t, http.StatusForbidden, w.Code)
	w = cmsTestRequest(mux, "GET", "/", nil, cookie)
	require.Equal(t, http.StatusOK, w.Code)
	require.NotContains(t, w.Body.String(), "/cms/login")
	for _, returned := range w.Result().Cookies() {
		require.NotEqual(t, cmsSessionCookie, returned.Name)
	}
	require.Equal(t, http.StatusSeeOther, cmsTestRequest(mux, "POST", "/cms/logout", url.Values{"csrf": {csrf}}, cookie).Code)
	require.Equal(t, http.StatusSeeOther, cmsTestRequest(mux, "GET", "/cms/posts", nil, cookie).Code)
	cookie, csrf = cmsTestLogin(t, mux)
	_, err := c.store.db.Exec(`UPDATE cms_sessions SET expires_at=?`, time.Now().Add(-time.Hour).Unix())
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, cmsTestRequest(mux, "POST", "/cms/logout", url.Values{"csrf": {csrf}}, cookie).Code)
	_, _ = cmsTestLogin(t, mux)
	_, oldHash, err := c.store.admin(context.Background())
	require.NoError(t, err)
	require.NoError(t, c.store.configureAdmin(context.Background(), "editor", cmsTestPassword, true))
	_, _, err = c.createSession(context.Background(), oldHash)
	require.ErrorIs(t, err, errCMSCredentialsChanged)
	var count int
	require.NoError(t, c.store.db.QueryRow(`SELECT COUNT(*) FROM cms_sessions`).Scan(&count))
	require.Zero(t, count)
}

func TestCMSLoginCSRFAndThrottle(t *testing.T) {
	_, mux := testCMS(t)
	require.Equal(t, http.StatusForbidden, cmsTestRequest(mux, "POST", "/cms/login", url.Values{"username": {"editor"}, "password": {cmsTestPassword}}).Code)
	w := cmsTestRequest(mux, "GET", "/cms/login", nil)
	cookie := cmsCookie(t, w, cmsLoginCookie)
	form := url.Values{"csrf": {cookie.Value}, "username": {"wrong-user"}, "password": {cmsTestPassword}}
	for i := 0; i < 10; i++ {
		w = cmsTestRequest(mux, "POST", "/cms/login", form, cookie)
		require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "Incorrect username or password.")
	}
	w = cmsTestRequest(mux, "POST", "/cms/login", form, cookie)
	require.Equal(t, http.StatusTooManyRequests, w.Code)
	require.Equal(t, "600", w.Header().Get("Retry-After"))
}

func TestCMSContentValidationAndStableURLs(t *testing.T) {
	c, _ := testCMS(t)
	for _, tc := range []struct {
		name   string
		modify func(*cmsPost)
	}{
		{"empty title", func(p *cmsPost) { p.Title = "" }},
		{"invalid slug", func(p *cmsPost) { p.Slug = "../admin" }},
		{"slug double hyphen", func(p *cmsPost) { p.Slug = "bad--slug" }},
		{"unknown language", func(p *cmsPost) { p.Language = "xx" }},
		{"unknown category", func(p *cmsPost) { p.Category = "Other" }},
		{"invalid status", func(p *cmsPost) { p.Status = "scheduled" }},
		{"unsafe image", func(p *cmsPost) { p.CoverURL = "javascript:alert(1)" }},
		{"image credentials", func(p *cmsPost) { p.CoverURL = "https://user:pass@example.com/image.png" }},
		{"oversized body", func(p *cmsPost) { p.Body = strings.Repeat("a", 100001) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := cmsTestPost()
			tc.modify(&p)
			_, err := c.store.savePost(context.Background(), p)
			require.ErrorIs(t, err, errCMSValidation)
		})
	}
	p := cmsTestPost()
	p.Status = "published"
	id, err := c.store.savePost(context.Background(), p)
	require.NoError(t, err)
	_, err = c.store.savePost(context.Background(), p)
	require.ErrorIs(t, err, errCMSSlugTaken)
	p, err = c.store.postByID(context.Background(), id)
	require.NoError(t, err)
	initialPublished := p.PublishedAt
	p.Slug = "changed-url"
	_, err = c.store.savePost(context.Background(), p)
	require.ErrorIs(t, err, errCMSValidation)
	p.Slug = "network-update"
	p.Body = "Edited body"
	_, err = c.store.savePost(context.Background(), p)
	require.NoError(t, err)
	p, err = c.store.postByID(context.Background(), id)
	require.NoError(t, err)
	require.Equal(t, initialPublished, p.PublishedAt)
}

func TestCMSSQLitePersistsAndBootstrapDoesNotResetPassword(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "private", "cms.db")
	store, err := openCMSStore(ctx, path)
	require.NoError(t, err)
	require.NoError(t, store.configureAdmin(ctx, "editor", cmsTestPassword, false))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	_, err = store.savePost(ctx, cmsTestPost())
	require.NoError(t, err)
	require.NoError(t, store.Close())
	store, err = openCMSStore(ctx, path)
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, store.configureAdmin(ctx, "other", "different-password-1234", false))
	username, hash, err := store.admin(ctx)
	require.NoError(t, err)
	require.Equal(t, "editor", username)
	require.NoError(t, bcrypt.CompareHashAndPassword(hash, []byte(cmsTestPassword)))
	p, err := store.postByID(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "network-update", p.Slug)
	secret := filepath.Join(t.TempDir(), "password")
	require.NoError(t, os.WriteFile(secret, []byte("  password with spaces  \n"), 0600))
	password, err := cmsAdminPassword(secret)
	require.NoError(t, err)
	require.Equal(t, "  password with spaces  ", password)
}

func TestCMSMarkdownRejectsUnsafeHTMLAndLinks(t *testing.T) {
	html, err := renderCMSMarkdown("## Hello\n\n<script>alert(1)</script>\n\n[x](javascript:alert(1))\n\n![x](data:text/html,bad)\n\n**Safe** [guide](/docs)")
	require.NoError(t, err)
	require.NotContains(t, string(html), "<script>")
	require.NotContains(t, string(html), "javascript:")
	require.NotContains(t, string(html), "data:text/html")
	require.Contains(t, string(html), "<strong>Safe</strong>")
	require.Contains(t, string(html), `href="/docs"`)
}

func TestCMSPaginationLanguagesAndSEO(t *testing.T) {
	c, mux := testCMS(t)
	for i := 0; i < 14; i++ {
		p := cmsTestPost()
		p.Slug = "article-" + strconv.Itoa(i)
		p.Status = "published"
		if i%2 == 0 {
			p.Language = "zh"
			p.Category = "Guides"
		}
		_, err := c.store.savePost(context.Background(), p)
		require.NoError(t, err)
	}
	for _, lang := range []string{"en", "zh", "fr"} {
		for _, path := range []string{"/blog", "/cms/login"} {
			w := cmsTestRequest(mux, "GET", path+"?lang="+lang, nil)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			require.Contains(t, w.Body.String(), `<html lang="`+lang+`">`)
		}
	}
	w := cmsTestRequest(mux, "GET", "/blog?page=2&lang=zh", nil)
	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `rel="canonical" href="https://congrid.net/blog?lang=zh&amp;page=2"`)
	require.Contains(t, w.Body.String(), `hreflang="fr" href="https://congrid.net/blog?lang=fr&amp;page=2"`)
	require.Equal(t, http.StatusNotFound, cmsTestRequest(mux, "GET", "/blog?page=3", nil).Code)
	require.Equal(t, http.StatusBadRequest, cmsTestRequest(mux, "GET", "/blog?page=-1", nil).Code)
	w = cmsTestRequest(mux, "GET", "/blog?content_lang=zh&category=Guides", nil)
	require.Contains(t, w.Body.String(), `name="robots" content="noindex, nofollow"`)
	require.Equal(t, 7, strings.Count(w.Body.String(), `class="card blog-card"`))
	w = cmsTestRequest(mux, "GET", "/robots.txt", nil)
	require.Contains(t, w.Body.String(), "Disallow: /cms")
	require.Contains(t, w.Body.String(), "https://congrid.net/sitemap.xml")
}
