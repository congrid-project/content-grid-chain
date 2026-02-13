package typespb

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// VerificationResultString returns the canonical string used in commit hashes.
func VerificationResultString(passed bool) string {
	if passed {
		return "pass"
	}
	return "fail"
}

// ComputeVerificationCommitHash returns sha256(domain|round_start|verifier|result|evidence_hash|nonce).
func ComputeVerificationCommitHash(domain string, roundStartUnix int64, verifier string, passed bool, evidenceHash, nonce string) string {
	parts := []string{
		strings.TrimSpace(strings.ToLower(domain)),
		strconv.FormatInt(roundStartUnix, 10),
		strings.TrimSpace(verifier),
		VerificationResultString(passed),
		strings.TrimSpace(evidenceHash),
		strings.TrimSpace(nonce),
	}
	payload := strings.Join(parts, "|")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
