package typespb

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

func testAddress(seed string) string {
	return sdk.AccAddress([]byte(seed)).String()
}

func TestMsgRevealVerificationValidateBasicBindsSimilarityEvidence(t *testing.T) {
	verifier := testAddress("similar-evidence-verifier")
	evidenceHash := ComputeVerificationEvidenceHash(8, 6, 15, "expected", "observed")
	msg := &MsgRevealVerification{
		Verifier:               verifier,
		Domain:                 "example.com",
		RoundStartUnix:         3600,
		Passed:                 true,
		EvidenceHash:           evidenceHash,
		Nonce:                  "nonce",
		ObservedSimilarDomains: 8,
		MatchedSimilarDomains:  6,
		ExpectedSimilarDomains: 15,
		ExpectedSetHash:        "expected",
		ObservedSetHash:        "observed",
	}
	require.NoError(t, msg.ValidateBasic())

	msg.MatchedSimilarDomains = 7
	require.ErrorContains(t, msg.ValidateBasic(), "evidence_hash does not match")
}

func TestMsgRevealVerificationValidateBasicAllowsLegacyEvidence(t *testing.T) {
	msg := &MsgRevealVerification{
		Verifier:       testAddress("legacy-verifier"),
		Domain:         "example.com",
		RoundStartUnix: 3600,
		Passed:         true,
		EvidenceHash:   "legacy-observed-set-hash",
		Nonce:          "nonce",
	}
	require.NoError(t, msg.ValidateBasic())
}

func TestVerificationCommitHashV2BindsVerificationOwner(t *testing.T) {
	verifier := testAddress("owner-bound-verifier")
	ownerA := testAddress("owner-a")
	ownerB := testAddress("owner-b")

	hashA := ComputeVerificationCommitHashV2("example.com", 3600, verifier, ownerA, true, "evidence", "nonce")
	hashB := ComputeVerificationCommitHashV2("example.com", 3600, verifier, ownerB, true, "evidence", "nonce")

	require.Len(t, hashA, 64)
	require.NotEqual(t, hashA, hashB)
	require.NotEqual(t, hashA, ComputeVerificationCommitHash("example.com", 3600, verifier, true, "evidence", "nonce"))
}
