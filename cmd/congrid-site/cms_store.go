package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var (
	errCMSValidation = errors.New("invalid CMS content")
	errCMSSlugTaken  = errors.New("article URL is already in use")
	errCMSConflict   = errors.New("article was changed by another editor")
	cmsSlugPattern   = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

type cmsStore struct{ db *sql.DB }

type cmsPost struct {
	ID             int64
	Slug           string
	Title          string
	Summary        string
	Body           string
	Category       string
	Language       string
	Author         string
	CoverURL       string
	SEOTitle       string
	SEODescription string
	Status         string
	PublishedAt    int64
	CreatedAt      int64
	UpdatedAt      int64
	Revision       int64
}

func (p cmsPost) URL() string { return "/blog/" + p.Slug }
func (p cmsPost) PublishedDate() string {
	return time.Unix(p.PublishedAt, 0).UTC().Format("2006-01-02")
}
func (p cmsPost) UpdatedDate() string { return time.Unix(p.UpdatedAt, 0).UTC().Format("2006-01-02") }
func (p cmsPost) MetaTitle() string {
	if p.SEOTitle != "" {
		return p.SEOTitle
	}
	return p.Title + " — Congrid"
}
func (p cmsPost) MetaDescription() string {
	if p.SEODescription != "" {
		return p.SEODescription
	}
	return p.Summary
}

func openCMSStore(ctx context.Context, dbPath string) (*cmsStore, error) {
	if strings.TrimSpace(dbPath) == "" || strings.HasPrefix(dbPath, "file:") {
		return nil, fmt.Errorf("CMS database must be a filesystem path")
	}
	if dbPath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
			return nil, fmt.Errorf("create CMS database directory: %w", err)
		}
		// Pre-create with restrictive permissions; never store CMS credentials in a public directory.
		f, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, fmt.Errorf("create CMS database: %w", err)
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
		if err := os.Chmod(dbPath, 0600); err != nil {
			return nil, err
		}
	}
	dsn := dbPath
	if dbPath != ":memory:" {
		absolute, err := filepath.Abs(dbPath)
		if err != nil {
			return nil, err
		}
		dsn = (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolute)}).String()
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open CMS database: %w", err)
	}
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		"PRAGMA journal_mode = WAL", "PRAGMA busy_timeout = 5000", "PRAGMA foreign_keys = ON",
		`CREATE TABLE IF NOT EXISTS cms_admin (
			id INTEGER PRIMARY KEY CHECK (id = 1), username TEXT NOT NULL UNIQUE, password_hash BLOB NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS cms_sessions (
			token_hash TEXT PRIMARY KEY, csrf_token TEXT NOT NULL, expires_at INTEGER NOT NULL,
			admin_id INTEGER NOT NULL REFERENCES cms_admin(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS cms_posts (
			id INTEGER PRIMARY KEY AUTOINCREMENT, slug TEXT NOT NULL UNIQUE,
			title TEXT NOT NULL, summary TEXT NOT NULL, body TEXT NOT NULL,
			category TEXT NOT NULL, language TEXT NOT NULL, author TEXT NOT NULL,
			cover_url TEXT NOT NULL, seo_title TEXT NOT NULL, seo_description TEXT NOT NULL,
			status TEXT NOT NULL CHECK (status IN ('draft','published')),
			published_at INTEGER NOT NULL DEFAULT 0, created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL, revision INTEGER NOT NULL DEFAULT 1
		)`,
		`CREATE INDEX IF NOT EXISTS cms_posts_public ON cms_posts(status, published_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS cms_sessions_expiry ON cms_sessions(expires_at)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("initialize CMS database: %w", err)
		}
	}
	return &cmsStore{db: db}, nil
}

func (s *cmsStore) Close() error { return s.db.Close() }

// A supplied bootstrap password only creates the first account. Rotation is explicit.
func (s *cmsStore) configureAdmin(ctx context.Context, username, password string, reset bool) error {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cms_admin`).Scan(&count); err != nil {
		return err
	}
	if count != 0 && !reset {
		return nil
	}
	if password == "" && !reset {
		return nil
	}
	username = strings.TrimSpace(username)
	if username == "" || utf8.RuneCountInString(username) > 80 || len(password) < 12 || len(password) > 72 {
		return fmt.Errorf("CMS username required (max 80 characters); password must be 12–72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash CMS password: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if reset {
		_, err = tx.ExecContext(ctx, `INSERT INTO cms_admin(id,username,password_hash) VALUES(1,?,?)
			ON CONFLICT(id) DO UPDATE SET username=excluded.username, password_hash=excluded.password_hash`, username, hash)
	} else {
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO cms_admin(id,username,password_hash) VALUES(1,?,?)`, username, hash)
	}
	if err != nil {
		return fmt.Errorf("configure CMS account: %w", err)
	}
	if reset {
		if _, err := tx.ExecContext(ctx, `DELETE FROM cms_sessions`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *cmsStore) admin(ctx context.Context) (string, []byte, error) {
	var username string
	var hash []byte
	err := s.db.QueryRowContext(ctx, `SELECT username,password_hash FROM cms_admin WHERE id=1`).Scan(&username, &hash)
	return username, hash, err
}

const cmsPostColumns = `id,slug,title,summary,body,category,language,author,cover_url,seo_title,seo_description,status,published_at,created_at,updated_at,revision`

func scanCMSPost(row interface{ Scan(...any) error }) (cmsPost, error) {
	var p cmsPost
	err := row.Scan(&p.ID, &p.Slug, &p.Title, &p.Summary, &p.Body, &p.Category, &p.Language, &p.Author,
		&p.CoverURL, &p.SEOTitle, &p.SEODescription, &p.Status, &p.PublishedAt, &p.CreatedAt, &p.UpdatedAt, &p.Revision)
	return p, err
}

func (s *cmsStore) postByID(ctx context.Context, id int64) (cmsPost, error) {
	return scanCMSPost(s.db.QueryRowContext(ctx, `SELECT `+cmsPostColumns+` FROM cms_posts WHERE id=?`, id))
}
func (s *cmsStore) publishedPost(ctx context.Context, slug string) (cmsPost, error) {
	return scanCMSPost(s.db.QueryRowContext(ctx, `SELECT `+cmsPostColumns+` FROM cms_posts WHERE slug=? AND status='published'`, slug))
}

type cmsPostFilter struct {
	Public   bool
	Language string
	Category string
	Limit    int
	Offset   int
}

func (s *cmsStore) listPosts(ctx context.Context, f cmsPostFilter) ([]cmsPost, int, error) {
	where := "1=1"
	var args []any
	if f.Public {
		where += " AND status='published'"
	}
	if f.Language != "" {
		where += " AND language=?"
		args = append(args, f.Language)
	}
	if f.Category != "" {
		where += " AND category=?"
		args = append(args, f.Category)
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cms_posts WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := "updated_at DESC,id DESC"
	if f.Public {
		order = "published_at DESC,id DESC"
	}
	query := `SELECT ` + cmsPostColumns + ` FROM cms_posts WHERE ` + where + ` ORDER BY ` + order
	if f.Limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, f.Limit, f.Offset)
	}
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var posts []cmsPost
	for rows.Next() {
		p, err := scanCMSPost(rows)
		if err != nil {
			return nil, 0, err
		}
		posts = append(posts, p)
	}
	return posts, total, rows.Err()
}

func validCMSCategory(c string) bool { return c == "News" || c == "Guides" || c == "Updates" }

func validateCMSPost(p cmsPost) error {
	for _, field := range []struct {
		name, value string
		max         int
		required    bool
	}{
		{"Title", p.Title, 200, true}, {"Summary", p.Summary, 500, true}, {"Markdown content", p.Body, 100000, true},
		{"Author", p.Author, 100, true}, {"SEO title", p.SEOTitle, 200, false}, {"SEO description", p.SEODescription, 500, false},
	} {
		if (field.required && strings.TrimSpace(field.value) == "") || utf8.RuneCountInString(field.value) > field.max {
			return fmt.Errorf("%w: %s is required and must be within %d characters", errCMSValidation, field.name, field.max)
		}
	}
	if len(p.Slug) > 120 || !cmsSlugPattern.MatchString(p.Slug) {
		return fmt.Errorf("%w: use a URL slug of lowercase letters, numbers and single hyphens (max 120 characters)", errCMSValidation)
	}
	if !validLanguage(p.Language) || !validCMSCategory(p.Category) || (p.Status != "draft" && p.Status != "published") {
		return fmt.Errorf("%w: select a valid language, category and publication status", errCMSValidation)
	}
	if p.CoverURL != "" && !validCMSImageURL(p.CoverURL) {
		return fmt.Errorf("%w: cover image must use an HTTPS URL or a /static/ path", errCMSValidation)
	}
	return nil
}

func validCMSImageURL(raw string) bool {
	if len(raw) > 2000 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return false
	}
	if u.Scheme == "https" && u.Host != "" {
		return true
	}
	return u.Scheme == "" && u.Host == "" && strings.HasPrefix(u.Path, "/static/") && !strings.Contains(u.Path, "..")
}

func (s *cmsStore) savePost(ctx context.Context, p cmsPost) (int64, error) {
	if err := validateCMSPost(p); err != nil {
		return 0, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var duplicate int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM cms_posts WHERE slug=? AND id<>?`, p.Slug, p.ID).Scan(&duplicate); err != nil {
		return 0, err
	}
	if duplicate > 0 {
		return 0, errCMSSlugTaken
	}
	now := time.Now().UTC().Unix()
	var published int64
	if p.ID != 0 {
		var oldSlug string
		var revision int64
		if err := tx.QueryRowContext(ctx, `SELECT slug,published_at,revision FROM cms_posts WHERE id=?`, p.ID).Scan(&oldSlug, &published, &revision); err != nil {
			return 0, err
		}
		if revision != p.Revision {
			return 0, errCMSConflict
		}
		// Stable published URLs preserve inbound links when content is edited or unpublished.
		if published != 0 && oldSlug != p.Slug {
			return 0, fmt.Errorf("%w: a previously published article's URL cannot be changed", errCMSValidation)
		}
	}
	if p.Status == "published" && published == 0 {
		published = now
	}
	args := []any{p.Slug, p.Title, p.Summary, p.Body, p.Category, p.Language, p.Author, p.CoverURL, p.SEOTitle, p.SEODescription, p.Status, published}
	if p.ID == 0 {
		args = append(args, now, now)
		result, err := tx.ExecContext(ctx, `INSERT INTO cms_posts(slug,title,summary,body,category,language,author,cover_url,seo_title,seo_description,status,published_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, args...)
		if err != nil {
			return 0, fmt.Errorf("create article: %w", err)
		}
		p.ID, err = result.LastInsertId()
		if err != nil {
			return 0, err
		}
	} else {
		args = append(args, now, p.ID)
		_, err := tx.ExecContext(ctx, `UPDATE cms_posts SET slug=?,title=?,summary=?,body=?,category=?,language=?,author=?,cover_url=?,seo_title=?,seo_description=?,status=?,published_at=?,updated_at=?,revision=revision+1 WHERE id=?`, args...)
		if err != nil {
			return 0, fmt.Errorf("update article: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return p.ID, nil
}

func (s *cmsStore) deletePost(ctx context.Context, id, revision int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM cms_posts WHERE id=? AND revision=?`, id, revision)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errCMSConflict
	}
	return nil
}
