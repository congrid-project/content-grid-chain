package registry

import (
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	typespb "content-grid-chain/x/registry/typespb"
)

func TestRevealVerificationPersistsBoundSimilarityEvidence(t *testing.T) {
	keeper, ctx := setupKeeper(t)
	verifier := sdk.AccAddress([]byte("reveal-metrics-verifier"))
	roundStart := int64(1000)
	require.NoError(t, keeper.SetAssignment(ctx, PublisherVerificationAssignment{
		RoundStartUnix: roundStart,
		Domain:         "example.com",
		StartAtUnix:    1100,
		DeadlineUnix:   1700,
		Verifiers:      []string{verifier.String()},
	}))

	evidenceHash := typespb.ComputeVerificationEvidenceHash(8, 6, 15, "expected", "observed")
	commitHash := typespb.ComputeVerificationCommitHash("example.com", roundStart, verifier.String(), true, evidenceHash, "nonce")
	keeper.SetCommit(ctx, roundStart, "example.com", verifier.String(), commitHash)
	ctx = ctx.WithBlockTime(time.Unix(1450, 0).UTC())

	server := NewMsgServerImpl(keeper)
	_, err := server.RevealVerification(sdk.WrapSDKContext(ctx), &typespb.MsgRevealVerification{
		Verifier:               verifier.String(),
		Domain:                 "example.com",
		RoundStartUnix:         roundStart,
		Passed:                 true,
		EvidenceHash:           evidenceHash,
		Nonce:                  "nonce",
		ObservedSimilarDomains: 8,
		MatchedSimilarDomains:  6,
		ExpectedSimilarDomains: 15,
		ExpectedSetHash:        "expected",
		ObservedSetHash:        "observed",
	})
	require.NoError(t, err)

	submission, found := keeper.GetSubmission(ctx, roundStart, "example.com", verifier.String())
	require.True(t, found)
	require.EqualValues(t, 8, submission.ObservedSimilarDomains)
	require.EqualValues(t, 6, submission.MatchedSimilarDomains)
	require.EqualValues(t, 15, submission.ExpectedSimilarDomains)
	require.Equal(t, "expected", submission.ExpectedSetHash)
	require.Equal(t, "observed", submission.ObservedSetHash)
}

func TestRevealVerificationRequiresAssignmentOwnerBinding(t *testing.T) {
	keeper, ctx := setupKeeper(t)
	verifier := sdk.AccAddress([]byte("owner-bound-verifier"))
	verificationOwner := sdk.AccAddress([]byte("candidate-publisher"))
	roundStart := int64(2000)
	require.NoError(t, keeper.SetAssignment(ctx, PublisherVerificationAssignment{
		RoundStartUnix:    roundStart,
		Domain:            "example.com",
		StartAtUnix:       2100,
		DeadlineUnix:      2700,
		Verifiers:         []string{verifier.String()},
		VerificationOwner: verificationOwner.String(),
		Reregistration:    true,
	}))
	ctx = ctx.WithBlockTime(time.Unix(2450, 0).UTC())
	server := NewMsgServerImpl(keeper)

	legacyHash := typespb.ComputeVerificationCommitHash("example.com", roundStart, verifier.String(), true, "", "nonce")
	keeper.SetCommit(ctx, roundStart, "example.com", verifier.String(), legacyHash)
	_, err := server.RevealVerification(sdk.WrapSDKContext(ctx), &typespb.MsgRevealVerification{
		Verifier:       verifier.String(),
		Domain:         "example.com",
		RoundStartUnix: roundStart,
		Passed:         true,
		Nonce:          "nonce",
	})
	require.ErrorContains(t, err, "verification owner mismatch")

	boundHash := typespb.ComputeVerificationCommitHashV2("example.com", roundStart, verifier.String(), verificationOwner.String(), true, "", "nonce")
	keeper.SetCommit(ctx, roundStart, "example.com", verifier.String(), boundHash)
	_, err = server.RevealVerification(sdk.WrapSDKContext(ctx), &typespb.MsgRevealVerification{
		Verifier:          verifier.String(),
		Domain:            "example.com",
		RoundStartUnix:    roundStart,
		Passed:            true,
		Nonce:             "nonce",
		VerificationOwner: verificationOwner.String(),
	})
	require.NoError(t, err)
}
