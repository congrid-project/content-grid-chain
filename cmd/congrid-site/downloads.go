package main

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const downloadsDirEnv = "CONGRID_SITE_DOWNLOADS_DIR"

var downloadFilenamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type downloadHandler struct {
	root string
}

func defaultDownloadsDir() string {
	if configured := strings.TrimSpace(os.Getenv(downloadsDirEnv)); configured != "" {
		return configured
	}
	return filepath.Join("cmd", "congrid-site", "downloads")
}

func newDownloadHandler(root string) (*downloadHandler, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("downloads directory required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve downloads directory: %w", err)
	}
	return &downloadHandler{root: absRoot}, nil
}

func (h *downloadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	filename := strings.TrimSpace(r.PathValue("filename"))
	if !validDownloadFilename(filename) {
		http.NotFound(w, r)
		return
	}

	filePath := filepath.Join(h.root, filename)
	info, err := os.Lstat(filePath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		http.NotFound(w, r)
		return
	}
	file, err := os.Open(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": filename})
	w.Header().Set("Content-Disposition", disposition)
	if filename == "seeds.txt" || filename == "seeds.txt.sha256" {
		w.Header().Set("Cache-Control", "no-cache")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, filename, info.ModTime(), file)
}

func validDownloadFilename(filename string) bool {
	return downloadFilenamePattern.MatchString(filename) && filepath.Base(filename) == filename
}
