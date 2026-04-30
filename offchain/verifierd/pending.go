package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	registrypb "content-grid-chain/x/registry/typespb"
)

type pendingReveal struct {
	Key            string `json:"key"`
	Domain         string `json:"domain"`
	RoundStartUnix int64  `json:"round_start_unix"`
	Verifier       string `json:"verifier"`
	Passed         bool   `json:"passed"`
	EvidenceHash   string `json:"evidence_hash"`
	Nonce          string `json:"nonce"`
	CommitHash     string `json:"commit_hash"`
	CommitRecorded bool   `json:"commit_recorded"`
	CreatedAtUnix  int64  `json:"created_at_unix"`
	UpdatedAtUnix  int64  `json:"updated_at_unix"`
}

func (a *Agent) pendingRevealForAssignment(assignment *registrypb.PublisherVerificationAssignment) (pendingReveal, bool, error) {
	if assignment == nil {
		return pendingReveal{}, false, nil
	}
	path := a.pendingRevealPath(assignment)
	if path == "" {
		return pendingReveal{}, false, nil
	}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return pendingReveal{}, false, nil
	}
	if err != nil {
		return pendingReveal{}, false, err
	}
	var pending pendingReveal
	if err := json.Unmarshal(b, &pending); err != nil {
		return pendingReveal{}, false, err
	}
	if err := pending.validateForAssignment(assignment, a.Cfg.VerifierAddress); err != nil {
		return pendingReveal{}, false, err
	}
	return pending, true, nil
}

func (a *Agent) savePendingReveal(pending pendingReveal) error {
	if strings.TrimSpace(a.Cfg.StateDir) == "" {
		return nil
	}
	now := time.Now().UTC().Unix()
	if pending.CreatedAtUnix == 0 {
		pending.CreatedAtUnix = now
	}
	pending.UpdatedAtUnix = now
	if err := os.MkdirAll(a.Cfg.StateDir, 0o700); err != nil {
		return err
	}
	path := a.pendingRevealPathForKey(pending.Key)
	if path == "" {
		return fmt.Errorf("state_dir required")
	}
	b, err := json.MarshalIndent(pending, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (a *Agent) deletePendingReveal(assignment *registrypb.PublisherVerificationAssignment) {
	path := a.pendingRevealPath(assignment)
	if path != "" {
		_ = os.Remove(path)
	}
}

func (a *Agent) pendingRevealPath(assignment *registrypb.PublisherVerificationAssignment) string {
	if assignment == nil {
		return ""
	}
	return a.pendingRevealPathForKey(assignmentKey(assignment))
}

func (a *Agent) pendingRevealPathForKey(key string) string {
	dir := strings.TrimSpace(a.Cfg.StateDir)
	if dir == "" || strings.TrimSpace(key) == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".json")
}

func (p pendingReveal) validateForAssignment(assignment *registrypb.PublisherVerificationAssignment, verifier string) error {
	if assignment == nil {
		return fmt.Errorf("assignment required")
	}
	if p.Key != assignmentKey(assignment) {
		return fmt.Errorf("pending key mismatch")
	}
	if p.Domain != assignment.GetDomain() || p.RoundStartUnix != assignment.GetRoundStartUnix() {
		return fmt.Errorf("pending assignment mismatch")
	}
	if strings.TrimSpace(p.Verifier) != strings.TrimSpace(verifier) {
		return fmt.Errorf("pending verifier mismatch")
	}
	if strings.TrimSpace(p.Nonce) == "" {
		return fmt.Errorf("pending nonce missing")
	}
	if strings.TrimSpace(p.CommitHash) == "" {
		return fmt.Errorf("pending commit hash missing")
	}
	return nil
}
