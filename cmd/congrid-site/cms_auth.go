package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const (
	cmsSessionCookie   = "congrid_cms_session"
	cmsLoginCookie     = "congrid_cms_login_csrf"
	cmsSessionLifetime = 12 * time.Hour
	cmsMaxBody         = 1024 * 1024
)

var errCMSCredentialsChanged = errors.New("CMS credentials changed during sign-in")

type cmsSession struct {
	TokenHash string
	CSRF      string
	ExpiresAt int64
	Username  string
}

type cmsLoginBucket struct {
	Count int
	Until time.Time
}
type cmsLoginLimiter struct {
	sync.Mutex
	buckets map[string]cmsLoginBucket
}

func (l *cmsLoginLimiter) allow(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	l.Lock()
	defer l.Unlock()
	if l.buckets == nil {
		l.buckets = make(map[string]cmsLoginBucket)
	}
	now := time.Now()
	for key, bucket := range l.buckets {
		if !now.Before(bucket.Until) {
			delete(l.buckets, key)
		}
	}
	bucket, ok := l.buckets[host]
	if !ok {
		if len(l.buckets) >= 4096 {
			return false
		}
		bucket.Until = now.Add(10 * time.Minute)
	}
	if bucket.Count >= 10 {
		return false
	}
	bucket.Count++
	l.buckets[host] = bucket
	return true
}

func cmsRandomToken() (string, error) {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(token[:]), nil
}

func cmsTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func cmsTokensEqual(a, b string) bool {
	return a != "" && b != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func cmsAdminPassword(file string) (string, error) {
	if file == "" {
		return os.Getenv("CONGRID_CMS_ADMIN_PASSWORD"), nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("read CMS password file: %w", err)
	}
	return strings.TrimRight(string(data), "\r\n"), nil
}

func (c *cmsApp) secureCookie(r *http.Request) bool {
	return strings.HasPrefix(c.baseURL, "https://") || r.TLS != nil
}

func (c *cmsApp) setCookie(w http.ResponseWriter, r *http.Request, name, value string, age int) {
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: value, Path: "/cms", MaxAge: age,
		Expires:  time.Now().Add(time.Duration(age) * time.Second),
		HttpOnly: true, Secure: c.secureCookie(r), SameSite: http.SameSiteStrictMode,
	})
}

func (c *cmsApp) session(r *http.Request) (cmsSession, error) {
	var session cmsSession
	cookie, err := r.Cookie(cmsSessionCookie)
	if err != nil || len(cookie.Value) != 43 {
		return session, sql.ErrNoRows
	}
	session.TokenHash = cmsTokenHash(cookie.Value)
	err = c.store.db.QueryRowContext(r.Context(), `SELECT s.csrf_token,s.expires_at,a.username FROM cms_sessions s JOIN cms_admin a ON a.id=s.admin_id WHERE s.token_hash=? AND s.expires_at>?`, session.TokenHash, time.Now().Unix()).Scan(&session.CSRF, &session.ExpiresAt, &session.Username)
	return session, err
}

func (c *cmsApp) createSession(ctx context.Context, authenticatedHash []byte) (string, cmsSession, error) {
	var session cmsSession
	token, err := cmsRandomToken()
	if err != nil {
		return "", session, err
	}
	csrf, err := cmsRandomToken()
	if err != nil {
		return "", session, err
	}
	session = cmsSession{TokenHash: cmsTokenHash(token), CSRF: csrf, ExpiresAt: time.Now().Add(cmsSessionLifetime).Unix()}
	tx, err := c.store.db.BeginTx(ctx, nil)
	if err != nil {
		return "", session, err
	}
	defer tx.Rollback()
	// A concurrent password reset must also invalidate a sign-in already in progress.
	var currentHash []byte
	if err := tx.QueryRowContext(ctx, `SELECT password_hash FROM cms_admin WHERE id=1`).Scan(&currentHash); err != nil {
		return "", session, err
	}
	if !bytes.Equal(currentHash, authenticatedHash) {
		return "", session, errCMSCredentialsChanged
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM cms_sessions WHERE expires_at<=?`, time.Now().Unix()); err != nil {
		return "", session, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO cms_sessions(token_hash,csrf_token,expires_at,admin_id) VALUES(?,?,?,1)`, session.TokenHash, csrf, session.ExpiresAt); err != nil {
		return "", session, err
	}
	return token, session, tx.Commit()
}

func cmsPrivateHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
}

func (c *cmsApp) validOrigin(r *http.Request) bool {
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(c.baseURL)
	return err == nil && origin == u.Scheme+"://"+u.Host
}

func (c *cmsApp) requireAdmin(next func(http.ResponseWriter, *http.Request, cmsSession)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cmsPrivateHeaders(w)
		session, err := c.session(r)
		if errors.Is(err, sql.ErrNoRows) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				http.Redirect(w, r, "/cms/login", http.StatusSeeOther)
			} else {
				http.Error(w, "CMS sign-in required", http.StatusUnauthorized)
			}
			return
		}
		if err != nil {
			c.internalError(w, err)
			return
		}
		if r.Method == http.MethodPost {
			r.Body = http.MaxBytesReader(w, r.Body, cmsMaxBody)
			if err := r.ParseForm(); err != nil {
				http.Error(w, "invalid or oversized form", http.StatusBadRequest)
				return
			}
			if !c.validOrigin(r) || !cmsTokensEqual(session.CSRF, r.PostForm.Get("csrf")) {
				http.Error(w, "invalid CSRF token", http.StatusForbidden)
				return
			}
		}
		next(w, r, session)
	}
}

func (c *cmsApp) handleLoginGet(w http.ResponseWriter, r *http.Request) {
	cmsPrivateHeaders(w)
	if _, err := c.session(r); err == nil {
		http.Redirect(w, r, "/cms/posts", http.StatusSeeOther)
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		c.internalError(w, err)
		return
	}
	token, err := cmsRandomToken()
	if err != nil {
		c.internalError(w, err)
		return
	}
	c.setCookie(w, r, cmsLoginCookie, token, 600)
	data := cmsView{pageData: c.adminPage("CMS sign in", "/cms/login"), CSRF: token}
	if _, _, err := c.store.admin(r.Context()); errors.Is(err, sql.ErrNoRows) {
		data.Error = "CMS administrator has not been configured. Set CONGRID_CMS_ADMIN_PASSWORD and restart the service."
	} else if err != nil {
		c.internalError(w, err)
		return
	}
	c.srv.render(w, r, "cms-login.html", data)
}

func (c *cmsApp) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	cmsPrivateHeaders(w)
	r.Body = http.MaxBytesReader(w, r.Body, cmsMaxBody)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid or oversized form", http.StatusBadRequest)
		return
	}
	cookie, err := r.Cookie(cmsLoginCookie)
	if err != nil || len(cookie.Value) != 43 || !cmsTokensEqual(cookie.Value, r.PostForm.Get("csrf")) || !c.validOrigin(r) {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	if !c.limiter.allow(r.RemoteAddr) {
		w.Header().Set("Retry-After", "600")
		http.Error(w, "too many sign-in attempts; try again later", http.StatusTooManyRequests)
		return
	}
	select {
	case c.passwordChecks <- struct{}{}:
		defer func() { <-c.passwordChecks }()
	default:
		http.Error(w, "sign-in busy; try again later", http.StatusTooManyRequests)
		return
	}
	username, hash, err := c.store.admin(r.Context())
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		c.internalError(w, err)
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		hash = c.dummyHash
	}
	passwordOK := bcrypt.CompareHashAndPassword(hash, []byte(r.PostForm.Get("password"))) == nil
	if !passwordOK || err != nil || !cmsTokensEqual(username, strings.TrimSpace(r.PostForm.Get("username"))) {
		data := cmsView{pageData: c.adminPage("CMS sign in", "/cms/login"), CSRF: cookie.Value, Error: "Incorrect username or password."}
		c.srv.renderStatus(w, r, "cms-login.html", data, http.StatusUnauthorized)
		return
	}
	token, _, err := c.createSession(r.Context(), hash)
	if errors.Is(err, errCMSCredentialsChanged) {
		http.Error(w, "credentials changed; sign in again", http.StatusUnauthorized)
		return
	}
	if err != nil {
		c.internalError(w, err)
		return
	}
	// Replace any previous session rather than keeping parallel tokens in this browser.
	if previous, err := r.Cookie(cmsSessionCookie); err == nil {
		if _, err := c.store.db.ExecContext(r.Context(), `DELETE FROM cms_sessions WHERE token_hash=?`, cmsTokenHash(previous.Value)); err != nil {
			c.internalError(w, err)
			return
		}
	}
	c.setCookie(w, r, cmsSessionCookie, token, int(cmsSessionLifetime.Seconds()))
	c.setCookie(w, r, cmsLoginCookie, "", -1)
	http.Redirect(w, r, "/cms/posts", http.StatusSeeOther)
}

func (c *cmsApp) handleLogout(w http.ResponseWriter, r *http.Request, session cmsSession) {
	if _, err := c.store.db.ExecContext(r.Context(), `DELETE FROM cms_sessions WHERE token_hash=?`, session.TokenHash); err != nil {
		c.internalError(w, err)
		return
	}
	c.setCookie(w, r, cmsSessionCookie, "", -1)
	http.Redirect(w, r, "/cms/login", http.StatusSeeOther)
}
