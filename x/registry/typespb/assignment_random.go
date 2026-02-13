package typespb

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"strings"
)

// ComputeRoundSeed returns a deterministic round seed derived from chain id and round start.
//
// NOTE: this is kept for backwards compatibility with older rounds and clients.
func ComputeRoundSeed(chainID string, roundStartUnix int64) [32]byte {
	payload := strings.TrimSpace(chainID) + "|" + strconv.FormatInt(roundStartUnix, 10)
	return sha256.Sum256([]byte(payload))
}

// ComputeRoundSeedWithAnchor returns a deterministic round seed derived from
// chain id, round start, and anchor block material (height + hash).
func ComputeRoundSeedWithAnchor(chainID string, roundStartUnix, anchorHeight int64, anchorHash []byte) [32]byte {
	anchorHex := hex.EncodeToString(anchorHash)
	payload := strings.TrimSpace(chainID) + "|" + strconv.FormatInt(roundStartUnix, 10) + "|" + strconv.FormatInt(anchorHeight, 10) + "|" + anchorHex
	return sha256.Sum256([]byte(payload))
}

// ComputeRoundSeedWithDrand returns a deterministic round seed derived from
// chain anchor material plus drand beacon material.
func ComputeRoundSeedWithDrand(chainID string, roundStartUnix, anchorHeight int64, anchorHash []byte, drandRound uint64, drandRandomness []byte) [32]byte {
	anchorHex := hex.EncodeToString(anchorHash)
	drandHex := hex.EncodeToString(drandRandomness)
	payload := strings.TrimSpace(chainID) + "|" + strconv.FormatInt(roundStartUnix, 10) + "|" + strconv.FormatInt(anchorHeight, 10) + "|" + anchorHex + "|" + strconv.FormatUint(drandRound, 10) + "|" + drandHex
	return sha256.Sum256([]byte(payload))
}

// ComputeAssignmentOffsetSeconds returns the deterministic offset within a round for a domain.
//
// For hourly-or-longer rounds, this follows minute scheduling (0..59 minutes) as:
//
//	offset = (H(seed|domain) % 60) * 60
//
// For shorter rounds (e.g. fast local e2e), it falls back to second offsets constrained by
// assignmentDelayMaxSeconds (capped by roundIntervalSeconds).
func ComputeAssignmentOffsetSeconds(roundSeed [32]byte, domain string, roundIntervalSeconds, assignmentDelayMaxSeconds int64) int64 {
	normDomain := strings.TrimSpace(strings.ToLower(domain))
	buf := make([]byte, 0, len(roundSeed)+len(normDomain))
	buf = append(buf, roundSeed[:]...)
	buf = append(buf, normDomain...)
	domainSeed := sha256.Sum256(buf)
	raw := binary.BigEndian.Uint64(domainSeed[:8])

	if roundIntervalSeconds >= 3600 {
		minute := int64(raw % 60)
		return minute * 60
	}

	delayMax := assignmentDelayMaxSeconds
	if delayMax <= 0 || delayMax > roundIntervalSeconds {
		delayMax = roundIntervalSeconds
	}
	if delayMax <= 0 {
		return 0
	}
	return int64(raw % uint64(delayMax))
}

// ComputeAssignmentStartAtUnix returns the deterministic start time for a domain assignment.
func ComputeAssignmentStartAtUnix(roundSeed [32]byte, domain string, roundStartUnix, roundIntervalSeconds, assignmentDelayMaxSeconds int64) int64 {
	return roundStartUnix + ComputeAssignmentOffsetSeconds(roundSeed, domain, roundIntervalSeconds, assignmentDelayMaxSeconds)
}
