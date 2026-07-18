package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDownloadHandlerServesReleaseAndRanges(t *testing.T) {
	dir := t.TempDir()
	filename := "content-grid-d-linux-amd64.tar.gz"
	contents := []byte("release-archive")
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), contents, 0o644))

	handler, err := newDownloadHandler(dir)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle("GET /downloads/{filename}", handler)

	req := httptest.NewRequest(http.MethodGet, "/downloads/"+filename+"?checksum=sha256:test", nil)
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, contents, recorder.Body.Bytes())
	require.Equal(t, `attachment; filename=content-grid-d-linux-amd64.tar.gz`, recorder.Header().Get("Content-Disposition"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))

	rangeReq := httptest.NewRequest(http.MethodGet, "/downloads/"+filename, nil)
	rangeReq.Header.Set("Range", "bytes=0-6")
	rangeRecorder := httptest.NewRecorder()
	mux.ServeHTTP(rangeRecorder, rangeReq)
	require.Equal(t, http.StatusPartialContent, rangeRecorder.Code)
	require.Equal(t, contents[:7], rangeRecorder.Body.Bytes())
}

func TestDownloadHandlerSupportsHeadWithoutDirectoryListing(t *testing.T) {
	dir := t.TempDir()
	filename := "content-grid-d-linux-amd64.tar.gz"
	require.NoError(t, os.WriteFile(filepath.Join(dir, filename), []byte("archive"), 0o644))
	handler, err := newDownloadHandler(dir)
	require.NoError(t, err)
	mux := http.NewServeMux()
	mux.Handle("GET /downloads/{filename}", handler)

	headReq := httptest.NewRequest(http.MethodHead, "/downloads/"+filename, nil)
	headRecorder := httptest.NewRecorder()
	mux.ServeHTTP(headRecorder, headReq)
	require.Equal(t, http.StatusOK, headRecorder.Code)
	require.Equal(t, "7", headRecorder.Header().Get("Content-Length"))
	body, err := io.ReadAll(headRecorder.Result().Body)
	require.NoError(t, err)
	require.Empty(t, body)

	rootReq := httptest.NewRequest(http.MethodGet, "/downloads/", nil)
	rootRecorder := httptest.NewRecorder()
	mux.ServeHTTP(rootRecorder, rootReq)
	require.Equal(t, http.StatusNotFound, rootRecorder.Code)
}

func TestDownloadHandlerRejectsUnsafeAndNonRegularFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "directory"), 0o755))
	handler, err := newDownloadHandler(dir)
	require.NoError(t, err)

	for _, filename := range []string{"../secret", ".hidden", "nested/file", "directory", "name with spaces.tar.gz"} {
		t.Run(filename, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/downloads/placeholder", nil)
			req.SetPathValue("filename", filename)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, req)
			require.Equal(t, http.StatusNotFound, recorder.Code)
		})
	}
}

func TestDownloadHandlerRejectsSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires additional privileges on Windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target.tar.gz")
	require.NoError(t, os.WriteFile(target, []byte("archive"), 0o644))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "release.tar.gz")))

	handler, err := newDownloadHandler(dir)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodGet, "/downloads/release.tar.gz", nil)
	req.SetPathValue("filename", "release.tar.gz")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	require.Equal(t, http.StatusNotFound, recorder.Code)
}
