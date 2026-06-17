package main

import "testing"

func TestDefaultContentGridBinUsesCommandName(t *testing.T) {
	t.Setenv("CONTENT_GRID_BIN", "")

	if got, want := defaultContentGridBin(), defaultContentGridBinary; got != want {
		t.Fatalf("unexpected binary: got %q want %q", got, want)
	}
}

func TestDefaultContentGridBinUsesEnvironmentOverride(t *testing.T) {
	t.Setenv("CONTENT_GRID_BIN", " /usr/local/bin/content-grid-d ")

	if got, want := defaultContentGridBin(), "/usr/local/bin/content-grid-d"; got != want {
		t.Fatalf("unexpected binary: got %q want %q", got, want)
	}
}
