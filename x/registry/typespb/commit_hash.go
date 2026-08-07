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

// ComputeVerificationCommitHashV2 binds the assignment-scoped owner that the
// verifier checked. Assignments created before publisher-rewards-v3 have no
// verification owner and continue to use the legacy hash above.
func ComputeVerificationCommitHashV2(domain string, roundStartUnix int64, verifier, verificationOwner string, passed bool, evidenceHash, nonce string) string {
	parts := []string{
		"publisher-verification-v2",
		strings.TrimSpace(strings.ToLower(domain)),
		strconv.FormatInt(roundStartUnix, 10),
		strings.TrimSpace(verifier),
		strings.TrimSpace(verificationOwner),
		VerificationResultString(passed),
		strings.TrimSpace(evidenceHash),
		strings.TrimSpace(nonce),
	}
	payload := strings.Join(parts, "|")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// ComputeVerificationEvidenceHash binds the similar-site observations revealed
// after the commit window. Badge verification remains the independent pass/fail
// result; this evidence only determines the publisher payout multiplier.
func ComputeVerificationEvidenceHash(observed, matched, expected int32, expectedSetHash, observedSetHash string) string {
	parts := []string{
		"publisher-similar-v1",
		strconv.FormatInt(int64(observed), 10),
		strconv.FormatInt(int64(matched), 10),
		strconv.FormatInt(int64(expected), 10),
		strings.TrimSpace(strings.ToLower(expectedSetHash)),
		strings.TrimSpace(strings.ToLower(observedSetHash)),
	}
	payload := strings.Join(parts, "|")
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}

// HasVerificationEvidence reports whether a reveal contains the new structured
// similar-site evidence. All-zero legacy reveals intentionally remain valid.
func HasVerificationEvidence(observed, matched, expected int32, expectedSetHash, observedSetHash string) bool {
	return observed != 0 || matched != 0 || expected != 0 ||
		strings.TrimSpace(expectedSetHash) != "" || strings.TrimSpace(observedSetHash) != ""
}
