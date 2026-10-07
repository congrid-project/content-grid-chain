package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"golang.org/x/crypto/bcrypt"
)

type cmsApp struct {
	srv            *server
	store          *cmsStore
	baseURL        string
	limiter        cmsLoginLimiter
	passwordChecks chan struct{}
	dummyHash      []byte
}

type cmsView struct {
	pageData
	CSRF          string
	Username      string
	Error         string
	Notice        string
	Post          cmsPost
	Posts         []cmsPost
	HTML          template.HTML
	Category      string
	ContentFilter string
	Page          int
	Total         int
	PreviousURL   string
	NextURL       string
	Preview       bool
}

func newCMSApp(srv *server, store *cmsStore, baseURL string) (*cmsApp, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("CMS base URL must be an HTTP(S) origin without a path, credentials or query")
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte("unused-cms-password-timing-check"), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	return &cmsApp{srv: srv, store: store, baseURL: u.Scheme + "://" + u.Host, passwordChecks: make(chan struct{}, 4), dummyHash: dummy}, nil
}

func (c *cmsApp) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /blog", c.handleBlog)
	mux.HandleFunc("GET /blog/{slug}", c.handleArticle)
	mux.HandleFunc("GET /feed.xml", c.handleFeed)
	mux.HandleFunc("GET /sitemap.xml", c.handleSitemap)
	mux.HandleFunc("GET /robots.txt", c.handleRobots)
	mux.HandleFunc("GET /cms", c.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ cmsSession) {
		http.Redirect(w, r, "/cms/posts", http.StatusSeeOther)
	}))
	mux.HandleFunc("GET /cms/{$}", c.requireAdmin(func(w http.ResponseWriter, r *http.Request, _ cmsSession) {
		http.Redirect(w, r, "/cms/posts", http.StatusSeeOther)
	}))
	mux.HandleFunc("GET /cms/login", c.handleLoginGet)
	mux.HandleFunc("POST /cms/login", c.handleLoginPost)
	mux.HandleFunc("POST /cms/logout", c.requireAdmin(c.handleLogout))
	mux.HandleFunc("GET /cms/posts", c.requireAdmin(c.handlePosts))
	mux.HandleFunc("GET /cms/posts/new", c.requireAdmin(c.handleNewPost))
	mux.HandleFunc("POST /cms/posts/new", c.requireAdmin(c.handleSavePost))
	mux.HandleFunc("GET /cms/posts/{id}/edit", c.requireAdmin(c.handleEditPost))
	mux.HandleFunc("POST /cms/posts/{id}/edit", c.requireAdmin(c.handleSavePost))
	mux.HandleFunc("GET /cms/posts/{id}/preview", c.requireAdmin(c.handlePreview))
	mux.HandleFunc("POST /cms/posts/{id}/delete", c.requireAdmin(c.handleDeletePost))
}

func (c *cmsApp) adminPage(title, path string) pageData {
	return pageData{Title: title, BaseURL: c.baseURL, Path: path, NoIndex: true}
}

func (c *cmsApp) internalError(w http.ResponseWriter, err error) {
	log.Printf("CMS: %v", err)
	http.Error(w, "content service unavailable", http.StatusInternalServerError)
}

func cmsPageNumber(r *http.Request) (int, error) {
	if r.URL.Query().Get("page") == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 1 || n > 1000000 {
		return 0, fmt.Errorf("invalid page number")
	}
	return n, nil
}

func cmsPaginationURL(r *http.Request, page int) string {
	u := *r.URL
	q := u.Query()
	q.Set("page", strconv.Itoa(page))
	u.RawQuery = q.Encode()
	return u.RequestURI()
}

func (c *cmsApp) handleBlog(w http.ResponseWriter, r *http.Request) {
	page, err := cmsPageNumber(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	category := r.URL.Query().Get("category")
	lang := r.URL.Query().Get("content_lang")
	if (category != "" && !validCMSCategory(category)) || (lang != "" && !validLanguage(lang)) {
		http.Error(w, "invalid article filter", http.StatusBadRequest)
		return
	}
	const pageSize = 12
	posts, total, err := c.store.listPosts(r.Context(), cmsPostFilter{Public: true, Category: category, Language: lang, Limit: pageSize, Offset: (page - 1) * pageSize})
	if err != nil {
		c.internalError(w, err)
		return
	}
	if page > 1 && len(posts) == 0 {
		http.NotFound(w, r)
		return
	}
	canonical := seoCanonicalURL(c.baseURL, "/blog", requestLanguage(r))
	if page > 1 {
		u, _ := url.Parse(canonical)
		q := u.Query()
		q.Set("page", strconv.Itoa(page))
		u.RawQuery = q.Encode()
		canonical = u.String()
	}
	data := cmsView{pageData: pageData{
		Title: "Blog — Congrid", Description: "News, practical guides and protocol updates from the Congrid network.",
		BaseURL: c.baseURL, Path: "/blog", CanonicalURL: canonical, NoIndex: category != "" || lang != "",
	}, Posts: posts, Total: total, Page: page, Category: category, ContentFilter: lang}
	if page > 1 {
		data.PreviousURL = cmsPaginationURL(r, page-1)
	}
	if page*pageSize < total {
		data.NextURL = cmsPaginationURL(r, page+1)
	}
	c.srv.render(w, r, "blog.html", data)
}

var cmsMarkdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

func renderCMSMarkdown(body string) (template.HTML, error) {
	var buf bytes.Buffer
	// Goldmark's default renderer blocks raw HTML and dangerous URL schemes.
	if err := cmsMarkdown.Convert([]byte(body), &buf); err != nil {
		return "", err
	}
	return template.HTML(buf.String()), nil
}

func (c *cmsApp) articleView(p cmsPost, preview bool) (cmsView, error) {
	html, err := renderCMSMarkdown(p.Body)
	if err != nil {
		return cmsView{}, err
	}
	data := cmsView{pageData: pageData{
		Title: p.MetaTitle(), Description: p.MetaDescription(), BaseURL: c.baseURL, Path: "/blog",
		CanonicalURL: c.baseURL + p.URL(), RawMetadata: true, ContentLanguage: p.Language,
		OGType: "article", OGImage: c.absoluteImageURL(p.CoverURL), NoIndex: preview,
	}, Post: p, HTML: html, Preview: preview}
	if !preview {
		data.ArticlePublished = seoTimestamp(p.PublishedAt)
		data.ArticleModified = seoTimestamp(p.UpdatedAt)
		data.StructuredData = map[string]any{
			"@context": "https://schema.org", "@type": "BlogPosting", "headline": p.Title,
			"description": p.MetaDescription(), "inLanguage": p.Language, "url": c.baseURL + p.URL(),
			"mainEntityOfPage": c.baseURL + p.URL(), "datePublished": seoTimestamp(p.PublishedAt), "dateModified": seoTimestamp(p.UpdatedAt),
			"author":    map[string]string{"@type": "Person", "name": p.Author},
			"publisher": map[string]string{"@type": "Organization", "name": "Congrid", "url": c.baseURL},
		}
		if p.CoverURL != "" {
			data.StructuredData.(map[string]any)["image"] = data.OGImage
		}
	}
	return data, nil
}

func (c *cmsApp) absoluteImageURL(image string) string {
	if strings.HasPrefix(image, "/static/") {
		return c.baseURL + image
	}
	return image
}

func (c *cmsApp) handleArticle(w http.ResponseWriter, r *http.Request) {
	post, err := c.store.publishedPost(r.Context(), r.PathValue("slug"))
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		c.internalError(w, err)
		return
	}
	data, err := c.articleView(post, false)
	if err != nil {
		c.internalError(w, err)
		return
	}
	c.srv.render(w, r, "blog-article.html", data)
}

func (c *cmsApp) handlePosts(w http.ResponseWriter, r *http.Request, session cmsSession) {
	page, err := cmsPageNumber(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	posts, total, err := c.store.listPosts(r.Context(), cmsPostFilter{Limit: 20, Offset: (page - 1) * 20})
	if err != nil {
		c.internalError(w, err)
		return
	}
	data := cmsView{pageData: c.adminPage("Content management", "/cms/posts"), CSRF: session.CSRF, Username: session.Username, Posts: posts, Total: total, Page: page}
	if r.URL.Query().Get("saved") == "1" {
		data.Notice = "Article saved."
	}
	if r.URL.Query().Get("deleted") == "1" {
		data.Notice = "Article deleted."
	}
	if page > 1 {
		data.PreviousURL = cmsPaginationURL(r, page-1)
	}
	if page*20 < total {
		data.NextURL = cmsPaginationURL(r, page+1)
	}
	c.srv.render(w, r, "cms-posts.html", data)
}

func (c *cmsApp) handleNewPost(w http.ResponseWriter, r *http.Request, session cmsSession) {
	c.srv.render(w, r, "cms-editor.html", cmsView{pageData: c.adminPage("New article", r.URL.Path), CSRF: session.CSRF, Username: session.Username,
		Post: cmsPost{Author: "Congrid", Category: "News", Language: requestLanguage(r), Status: "draft"}})
}

func cmsPostID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		return 0, sql.ErrNoRows
	}
	return id, nil
}

func (c *cmsApp) loadPost(w http.ResponseWriter, r *http.Request) (cmsPost, bool) {
	id, err := cmsPostID(r)
	if err != nil {
		http.NotFound(w, r)
		return cmsPost{}, false
	}
	p, err := c.store.postByID(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return cmsPost{}, false
	}
	if err != nil {
		c.internalError(w, err)
		return cmsPost{}, false
	}
	return p, true
}

func (c *cmsApp) handleEditPost(w http.ResponseWriter, r *http.Request, session cmsSession) {
	p, ok := c.loadPost(w, r)
	if !ok {
		return
	}
	data := cmsView{pageData: c.adminPage("Edit article", r.URL.Path), CSRF: session.CSRF, Username: session.Username, Post: p}
	if r.URL.Query().Get("saved") == "1" {
		data.Notice = "Article saved."
	}
	c.srv.render(w, r, "cms-editor.html", data)
}

func (c *cmsApp) handleSavePost(w http.ResponseWriter, r *http.Request, session cmsSession) {
	p := cmsPost{}
	if r.PathValue("id") != "" {
		var ok bool
		p, ok = c.loadPost(w, r)
		if !ok {
			return
		}
	}
	p.Slug = strings.TrimSpace(r.PostForm.Get("slug"))
	p.Title = strings.TrimSpace(r.PostForm.Get("title"))
	p.Summary = strings.TrimSpace(r.PostForm.Get("summary"))
	p.Body = r.PostForm.Get("body")
	p.Category = r.PostForm.Get("category")
	p.Language = r.PostForm.Get("language")
	p.Author = strings.TrimSpace(r.PostForm.Get("author"))
	p.CoverURL = strings.TrimSpace(r.PostForm.Get("cover_url"))
	p.SEOTitle = strings.TrimSpace(r.PostForm.Get("seo_title"))
	p.SEODescription = strings.TrimSpace(r.PostForm.Get("seo_description"))
	p.Status = r.PostForm.Get("status")
	p.Revision, _ = strconv.ParseInt(r.PostForm.Get("revision"), 10, 64)
	id, err := c.store.savePost(r.Context(), p)
	if errors.Is(err, errCMSValidation) || errors.Is(err, errCMSSlugTaken) || errors.Is(err, errCMSConflict) {
		message := err.Error()
		if errors.Is(err, errCMSConflict) {
			message = "Another editor changed this article. Open the saved version in a new tab and compare it with your content before saving."
		}
		c.srv.renderStatus(w, r, "cms-editor.html", cmsView{pageData: c.adminPage("Edit article", r.URL.Path), CSRF: session.CSRF, Username: session.Username, Post: p, Error: message}, http.StatusConflict)
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		c.internalError(w, err)
		return
	}
	http.Redirect(w, r, "/cms/posts/"+strconv.FormatInt(id, 10)+"/edit?saved=1", http.StatusSeeOther)
}

func (c *cmsApp) handlePreview(w http.ResponseWriter, r *http.Request, _ cmsSession) {
	p, ok := c.loadPost(w, r)
	if !ok {
		return
	}
	data, err := c.articleView(p, true)
	if err != nil {
		c.internalError(w, err)
		return
	}
	c.srv.render(w, r, "blog-article.html", data)
}

func (c *cmsApp) handleDeletePost(w http.ResponseWriter, r *http.Request, _ cmsSession) {
	id, err := cmsPostID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// A second confirmation form is rendered by the editor before this POST.
	if r.PostForm.Get("confirm") != "delete" {
		http.Error(w, "deletion confirmation required", http.StatusBadRequest)
		return
	}
	revision, _ := strconv.ParseInt(r.PostForm.Get("revision"), 10, 64)
	err = c.store.deletePost(r.Context(), id, revision)
	if errors.Is(err, errCMSConflict) {
		http.Error(w, "article changed; reload before deleting", http.StatusConflict)
		return
	}
	if err != nil {
		c.internalError(w, err)
		return
	}
	http.Redirect(w, r, "/cms/posts?deleted=1", http.StatusSeeOther)
}

func seoTimestamp(unix int64) string { return time.Unix(unix, 0).UTC().Format(time.RFC3339) }
