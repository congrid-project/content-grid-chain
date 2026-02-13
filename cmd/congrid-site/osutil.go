package main

import (
	"os"
	"path/filepath"
)

func osReadFileImpl(p string) ([]byte, error) {
	return os.ReadFile(p)
}

func osWriteFileAtomicImpl(p string, b []byte, perm uint32) error {
	tmp := p + ".tmp"
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, os.FileMode(perm)); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
