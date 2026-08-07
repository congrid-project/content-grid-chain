package registry

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	typespb "content-grid-chain/x/registry/typespb"
)

func TestRegisterPublisherCreatesPendingReregistrationWithoutReplacingOwner(t *testing.T) {
	keeper, ctx := setupKeeper(t)
	oldOwner := sdk.AccAddress([]byte("existing-publisher-owner"))
	newOwner := sdk.AccAddress([]byte("candidate-publisher-owner"))
	referrer := sdk.AccAddress([]byte("candidate-referrer"))
	require.NoError(t, keeper.UpsertWebsite(ctx, Website{
		Domain:   "example.com",
		Owner:    oldOwner.String(),
		Status:   StatusVerified,
		Referrer: oldOwner.String(),
	}))

	server := NewMsgServerImpl(keeper)
	_, err := server.RegisterPublisher(sdk.WrapSDKContext(ctx), &typespb.MsgRegisterPublisher{
		Owner:    newOwner.String(),
		Domain:   "example.com",
		Referrer: referrer.String(),
	})
	require.NoError(t, err)

	stored, found := keeper.GetWebsite(ctx, "example.com")
	require.True(t, found)
	require.Equal(t, oldOwner.String(), stored.Owner)
	require.Equal(t, StatusVerified, stored.Status)
	require.Equal(t, newOwner.String(), stored.PendingOwner)
	require.Equal(t, referrer.String(), stored.PendingReferrer)
}

func TestReregistrationRejectsCandidateReplacementButAllowsIncumbentCancellation(t *testing.T) {
	keeper, ctx := setupKeeper(t)
	oldOwner := sdk.AccAddress([]byte("existing-publisher-owner"))
	firstCandidate := sdk.AccAddress([]byte("first-candidate-owner"))
	otherCandidate := sdk.AccAddress([]byte("other-candidate-owner"))
	require.NoError(t, keeper.UpsertWebsite(ctx, Website{
		Domain:       "example.com",
		Owner:        oldOwner.String(),
		Status:       StatusVerified,
		PendingOwner: firstCandidate.String(),
	}))
	server := NewMsgServerImpl(keeper)

	_, err := server.RegisterPublisher(sdk.WrapSDKContext(ctx), &typespb.MsgRegisterPublisher{
		Owner:  otherCandidate.String(),
		Domain: "example.com",
	})
	require.ErrorContains(t, err, "pending for another owner")

	_, err = server.RegisterPublisher(sdk.WrapSDKContext(ctx), &typespb.MsgRegisterPublisher{
		Owner:  oldOwner.String(),
		Domain: "example.com",
	})
	require.NoError(t, err)
	stored, found := keeper.GetWebsite(ctx, "example.com")
	require.True(t, found)
	require.Equal(t, oldOwner.String(), stored.PendingOwner)
}

func TestReregistrationAppliesOwnerAndReferrerOnlyAfterVerification(t *testing.T) {
	keeper, ctx := setupKeeper(t)
	oldOwner := sdk.AccAddress([]byte("existing-publisher-owner"))
	newOwner := sdk.AccAddress([]byte("candidate-publisher-owner"))
	referrer := sdk.AccAddress([]byte("candidate-referrer"))
	verifier := sdk.AccAddress([]byte("reregistration-verifier"))
	require.NoError(t, keeper.UpsertWebsite(ctx, Website{
		Domain:          "example.com",
		Owner:           oldOwner.String(),
		Status:          StatusVerified,
		PendingOwner:    newOwner.String(),
		PendingReferrer: referrer.String(),
	}))
	require.NoError(t, keeper.SetAssignment(ctx, PublisherVerificationAssignment{
		RoundStartUnix:    900,
		Domain:            "example.com",
		StartAtUnix:       1000,
		DeadlineUnix:      1100,
		Verifiers:         []string{verifier.String()},
		VerificationOwner: newOwner.String(),
		Reregistration:    true,
	}))
	require.NoError(t, keeper.SetSubmission(ctx, PublisherVerificationSubmission{
		RoundStartUnix:  900,
		Domain:          "example.com",
		Verifier:        verifier.String(),
		Passed:          true,
		ObservedAtUnix:  1050,
		SubmittedAtUnix: 1050,
	}))
	require.NoError(t, keeper.SetSlot(ctx, Slot{
		ID:                 "slot-000001",
		Publisher:          oldOwner.String(),
		Domain:             "example.com",
		Label:              "Existing slot",
		RateDenom:          "ucongrid",
		RateAmount:         sdkmath.NewInt(1),
		UnitSeconds:        60,
		MinDurationSeconds: 60,
		MaxDurationSeconds: 600,
		Status:             SlotStatusPaused,
		CreatedAtUnix:      900,
		UpdatedAtUnix:      900,
	}))

	ctx = ctx.WithBlockHeight(10).WithBlockTime(time.Unix(1200, 0).UTC())
	require.NoError(t, keeper.finalizeAssignments(ctx))

	stored, found := keeper.GetWebsite(ctx, "example.com")
	require.True(t, found)
	require.Equal(t, newOwner.String(), stored.Owner)
	require.Equal(t, referrer.String(), stored.Referrer)
	require.Equal(t, StatusVerified, stored.Status)
	require.Empty(t, stored.PendingOwner)
	require.Empty(t, stored.PendingReferrer)
	require.EqualValues(t, 10, stored.RegisteredAtHeight)
	slot, found := keeper.GetSlot(ctx, "slot-000001")
	require.True(t, found)
	require.Equal(t, newOwner.String(), slot.Publisher)

	assignment, found := keeper.GetAssignment(ctx, 900, "example.com")
	require.True(t, found)
	require.True(t, assignment.Finalized)
	require.True(t, assignment.Verified)
}

func TestFailedReregistrationPreservesExistingRegistration(t *testing.T) {
	keeper, ctx := setupKeeper(t)
	oldOwner := sdk.AccAddress([]byte("existing-publisher-owner"))
	newOwner := sdk.AccAddress([]byte("candidate-publisher-owner"))
	verifier := sdk.AccAddress([]byte("reregistration-verifier"))
	require.NoError(t, keeper.UpsertWebsite(ctx, Website{
		Domain:       "example.com",
		Owner:        oldOwner.String(),
		Status:       StatusVerified,
		PendingOwner: newOwner.String(),
	}))
	require.NoError(t, keeper.SetAssignment(ctx, PublisherVerificationAssignment{
		RoundStartUnix:    900,
		Domain:            "example.com",
		StartAtUnix:       1000,
		DeadlineUnix:      1100,
		Verifiers:         []string{verifier.String()},
		VerificationOwner: newOwner.String(),
		Reregistration:    true,
	}))
	require.NoError(t, keeper.SetSubmission(ctx, PublisherVerificationSubmission{
		RoundStartUnix:  900,
		Domain:          "example.com",
		Verifier:        verifier.String(),
		Passed:          false,
		ObservedAtUnix:  1050,
		SubmittedAtUnix: 1050,
	}))

	ctx = ctx.WithBlockTime(time.Unix(1200, 0).UTC())
	require.NoError(t, keeper.finalizeAssignments(ctx))

	stored, found := keeper.GetWebsite(ctx, "example.com")
	require.True(t, found)
	require.Equal(t, oldOwner.String(), stored.Owner)
	require.Equal(t, StatusVerified, stored.Status)
	require.Empty(t, stored.PendingOwner)
	require.Zero(t, keeper.GetPublisherFailureStreak(ctx, "example.com"))
}

func TestAcceptedReregistrationPaysPublisherRewardToNewOwner(t *testing.T) {
	tokenomics := newRecordingTokenomicsKeeper()
	keeper, ctx := setupRewardKeeper(t, tokenomics)
	oldOwner := sdk.AccAddress([]byte("existing-publisher-owner"))
	newOwner := sdk.AccAddress([]byte("candidate-publisher-owner"))
	verifier := sdk.AccAddress([]byte("reregistration-verifier"))
	require.NoError(t, keeper.UpsertWebsite(ctx, Website{
		Domain:       "example.com",
		Owner:        oldOwner.String(),
		Status:       StatusVerified,
		PendingOwner: newOwner.String(),
	}))
	require.NoError(t, keeper.SetAssignment(ctx, PublisherVerificationAssignment{
		RoundStartUnix:    900,
		Domain:            "example.com",
		StartAtUnix:       1000,
		DeadlineUnix:      1100,
		Verifiers:         []string{verifier.String()},
		VerificationOwner: newOwner.String(),
		Reregistration:    true,
	}))
	require.NoError(t, keeper.SetSubmission(ctx, PublisherVerificationSubmission{
		RoundStartUnix:  900,
		Domain:          "example.com",
		Verifier:        verifier.String(),
		Passed:          true,
		ObservedAtUnix:  1050,
		SubmittedAtUnix: 1050,
	}))

	ctx = ctx.WithBlockTime(time.Unix(1200, 0).UTC())
	require.NoError(t, keeper.finalizeAssignments(ctx))
	require.NoError(t, keeper.settleFinalizedVerificationRounds(ctx))
	require.True(t, tokenomics.sent[newOwner.String()].IsPositive())
	_, oldOwnerPaid := tokenomics.sent[oldOwner.String()]
	require.False(t, oldOwnerPaid)
}
