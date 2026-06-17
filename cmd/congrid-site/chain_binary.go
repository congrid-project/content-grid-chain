package main

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

const defaultContentGridBinary = "content-grid-d"

func defaultContentGridBin() string {
	if bin := strings.TrimSpace(os.Getenv("CONTENT_GRID_BIN")); bin != "" {
		return bin
	}
	return defaultContentGridBinary
}

func contentGridCommand(ctx context.Context, binary string, args ...string) *exec.Cmd {
	bin := strings.TrimSpace(binary)
	if bin == "" {
		bin = defaultContentGridBinary
	}
	return exec.CommandContext(ctx, bin, args...)
}
