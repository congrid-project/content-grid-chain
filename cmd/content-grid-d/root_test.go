package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnexpectedRelativeAppDBAllowsDatabaseInsideHome(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "data", "application.db")
	if err := os.MkdirAll(dbPath, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := unexpectedRelativeAppDB(home, home); got != "" {
		t.Fatalf("expected database inside home to be allowed, got %q", got)
	}
}

func TestUnexpectedRelativeAppDBRejectsDatabaseOutsideHome(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(cwd, "data", "application.db")
	if err := os.MkdirAll(dbPath, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := unexpectedRelativeAppDB(cwd, home); got != dbPath {
		t.Fatalf("expected %q, got %q", dbPath, got)
	}
}

func TestUnexpectedRelativeAppDBIgnoresMissingDatabase(t *testing.T) {
	if got := unexpectedRelativeAppDB(t.TempDir(), t.TempDir()); got != "" {
		t.Fatalf("expected missing database to be ignored, got %q", got)
	}
}
