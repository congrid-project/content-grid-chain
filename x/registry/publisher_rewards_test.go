package registry

import (
	"testing"
	"time"

	"cosmossdk.io/log"
	sdkmath "cosmossdk.io/math"
	store "cosmossdk.io/store"
	"cosmossdk.io/store/metrics"
	storetypes "cosmossdk.io/store/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/require"

	"github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
)

func TestComputeSimilarRoundStats_LinksDoNotChangeBadgePass(t *testing.T) {
	subs := []PublisherVerificationSubmission{
		{Passed: true, ExpectedSimilarDomains: 15, ExpectedSetHash: "set-a", MatchedSimilarDomains: 0},
		{Passed: true, ExpectedSimilarDomains: 15, ExpectedSetHash: "set-a", MatchedSimilarDomains: 5},
		{Passed: false, ExpectedSimilarDomains: 15, ExpectedSetHash: "set-a", MatchedSimilarDomains: 15},
	}

	stats := computeSimilarRoundStats(3, subs)
	require.Equal(t, 2, stats.Passes)
	require.Equal(t, 1, stats.Fails)
	require.Equal(t, 2, stats.Quorum)
	require.Equal(t, "set-a", stats.MajorityExpectedHash)
	require.EqualValues(t, 5, stats.VerifiedSimilarDomains)
}

func TestComputeSimilarRoundStats_RequiresQuorumOnExpectedSet(t *testing.T) {
	subs := []PublisherVerificationSubmission{
		{Passed: true, ExpectedSetHash: "set-a", MatchedSimilarDomains: 15},
		{Passed: true, ExpectedSetHash: "set-b", MatchedSimilarDomains: 15},
		{Passed: false},
	}

	stats := computeSimilarRoundStats(3, subs)
	require.Equal(t, 2, stats.Passes)
	require.Empty(t, stats.MajorityExpectedHash)
	require.Zero(t, stats.VerifiedSimilarDomains)
}

func TestPublisherClaimableAmount_UsesTenPercentFloor(t *testing.T) {
	tests := []struct {
		name    string
		matched int32
		want    int64
	}{
		{name: "zero links", matched: 0, want: 100},
		{name: "one link below floor", matched: 1, want: 100},
		{name: "two links proportional", matched: 2, want: 133},
		{name: "half links", matched: 8, want: 533},
		{name: "full links", matched: 15, want: 1000},
		{name: "links capped", matched: 20, want: 1000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := publisherClaimableAmount(sdkmath.NewInt(1000), tt.matched, 15, 1000)
			require.Equal(t, sdkmath.NewInt(tt.want), got)
		})
	}
}

func TestAssignNewRoundIncludesVerifiedPublisherWithoutLease(t *testing.T) {
	keeper, ctx := setupKeeper(t)
	params := keeper.GetParams(ctx)
	params.DrandEnabled = false
	params.DrandStrictMode = false
	require.NoError(t, keeper.SetParams(ctx, params))

	owner := sdk.AccAddress([]byte("verified-publisher-owner"))
	candidate := sdk.AccAddress([]byte("replacement-publisher-owner"))
	require.NoError(t, keeper.UpsertWebsite(ctx, Website{
		Domain:       "example.com",
		Owner:        owner.String(),
		Status:       StatusVerified,
		PendingOwner: candidate.String(),
	}))

	ctx = ctx.WithBlockTime(time.Unix(100, 0).UTC())
	require.NoError(t, keeper.assignNewRound(ctx))
	assignment, found := keeper.GetAssignment(ctx, 3600, "example.com")
	require.True(t, found)
	require.True(t, assignment.Reregistration)
	require.Equal(t, candidate.String(), assignment.VerificationOwner)
}

func TestAssignNewRoundIncludesRevokedReregistrationCandidate(t *testing.T) {
	keeper, ctx := setupKeeper(t)
	params := keeper.GetParams(ctx)
	params.DrandEnabled = false
	params.DrandStrictMode = false
	require.NoError(t, keeper.SetParams(ctx, params))

	owner := sdk.AccAddress([]byte("revoked-publisher-owner"))
	candidate := sdk.AccAddress([]byte("revoked-replacement-owner"))
	require.NoError(t, keeper.UpsertWebsite(ctx, Website{
		Domain:            "revoked.example",
		Owner:             owner.String(),
		Status:            StatusRevoked,
		CooldownUntilUnix: 99_999,
		PendingOwner:      candidate.String(),
	}))

	ctx = ctx.WithBlockTime(time.Unix(100, 0).UTC())
	require.NoError(t, keeper.assignNewRound(ctx))
	assignment, found := keeper.GetAssignment(ctx, 3600, "revoked.example")
	require.True(t, found)
	require.True(t, assignment.Reregistration)
	require.Equal(t, candidate.String(), assignment.VerificationOwner)
}

type recordingTokenomicsKeeper struct {
	sent    map[string]sdkmath.Int
	burned  sdkmath.Int
	ensured sdkmath.Int
}

func newRecordingTokenomicsKeeper() *recordingTokenomicsKeeper {
	return &recordingTokenomicsKeeper{
		sent:    map[string]sdkmath.Int{},
		burned:  sdkmath.ZeroInt(),
		ensured: sdkmath.ZeroInt(),
	}
}

func (m *recordingTokenomicsKeeper) MintAndSend(ctx sdk.Context, recipient sdk.AccAddress, coins sdk.Coins) error {
	return nil
}

func (m *recordingTokenomicsKeeper) MintAndBurn(ctx sdk.Context, coins sdk.Coins) error {
	return nil
}

func (m *recordingTokenomicsKeeper) EnsureEmissionPool(ctx sdk.Context, denom string, targetAmount sdkmath.Int) error {
	m.ensured = targetAmount
	return nil
}

func (m *recordingTokenomicsKeeper) SendFromPool(ctx sdk.Context, recipient sdk.AccAddress, coins sdk.Coins) error {
	amount := coins.AmountOf(verificationRewardDenom)
	current, found := m.sent[recipient.String()]
	if !found {
		current = sdkmath.ZeroInt()
	}
	m.sent[recipient.String()] = current.Add(amount)
	return nil
}

func (m *recordingTokenomicsKeeper) BurnFromPool(ctx sdk.Context, coins sdk.Coins) error {
	m.burned = m.burned.Add(coins.AmountOf(verificationRewardDenom))
	return nil
}

func setupRewardKeeper(t *testing.T, tokenomics *recordingTokenomicsKeeper) (Keeper, sdk.Context) {
	t.Helper()
	storeKey := storetypes.NewMemoryStoreKey(StoreKey)
	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeMemory, nil)
	require.NoError(t, stateStore.LoadLatestVersion())

	interfaceRegistry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(interfaceRegistry)
	var bankKeeper bankkeeper.Keeper
	keeper := NewKeeper(cdc, storeKey, mockVerifierKeeper{}, tokenomics, bankKeeper)
	ctx := sdk.NewContext(stateStore, types.Header{ChainID: "reward-test", Height: 1}, false, log.NewNopLogger())
	require.NoError(t, keeper.SetParams(ctx, DefaultPublisherParams()))
	return keeper, ctx
}

func TestSettleVerificationRound_SplitsAcrossActivePublishersAndOnlyOnce(t *testing.T) {
	tokenomics := newRecordingTokenomicsKeeper()
	keeper, ctx := setupRewardKeeper(t, tokenomics)

	roundStart := int64(3600)
	verifier := sdk.AccAddress([]byte("reward-round-verifier"))
	ownerZero := sdk.AccAddress([]byte("publisher-zero-links"))
	ownerFull := sdk.AccAddress([]byte("publisher-full-links"))
	ownerInactive := sdk.AccAddress([]byte("publisher-inactive"))

	for _, website := range []Website{
		{Domain: "zero.example", Owner: ownerZero.String(), Status: StatusVerified},
		{Domain: "full.example", Owner: ownerFull.String(), Status: StatusVerified},
	} {
		require.NoError(t, keeper.UpsertWebsite(ctx, website))
		require.NoError(t, keeper.SetAssignment(ctx, PublisherVerificationAssignment{
			RoundStartUnix:  roundStart,
			Domain:          website.Domain,
			StartAtUnix:     roundStart,
			DeadlineUnix:    roundStart + 600,
			Verifiers:       []string{verifier.String()},
			Finalized:       true,
			FinalizedAtUnix: roundStart + 601,
			Verified:        true,
		}))
	}
	require.NoError(t, keeper.UpsertWebsite(ctx, Website{
		Domain: "inactive.example",
		Owner:  ownerInactive.String(),
		Status: StatusPending,
	}))
	require.NoError(t, keeper.SetAssignment(ctx, PublisherVerificationAssignment{
		RoundStartUnix: roundStart,
		Domain:         "inactive.example",
		StartAtUnix:    roundStart,
		DeadlineUnix:   roundStart + 600,
		Verifiers:      []string{verifier.String()},
	}))

	require.NoError(t, keeper.SetSubmission(ctx, PublisherVerificationSubmission{
		RoundStartUnix:         roundStart,
		Domain:                 "zero.example",
		Verifier:               verifier.String(),
		Passed:                 true,
		ObservedAtUnix:         roundStart + 400,
		SubmittedAtUnix:        roundStart + 400,
		ExpectedSimilarDomains: 15,
		ExpectedSetHash:        "expected",
		ObservedSetHash:        "observed-zero",
	}))
	require.NoError(t, keeper.SetSubmission(ctx, PublisherVerificationSubmission{
		RoundStartUnix:         roundStart,
		Domain:                 "full.example",
		Verifier:               verifier.String(),
		Passed:                 true,
		ObservedAtUnix:         roundStart + 400,
		SubmittedAtUnix:        roundStart + 400,
		ObservedSimilarDomains: 15,
		MatchedSimilarDomains:  15,
		ExpectedSimilarDomains: 15,
		ExpectedSetHash:        "expected",
		ObservedSetHash:        "observed-full",
	}))

	// The active-publisher denominator is not known until every assignment in
	// the round has finalized, so an incomplete round must not pay early.
	require.NoError(t, keeper.settleFinalizedVerificationRounds(ctx))
	require.Empty(t, tokenomics.sent)
	inactiveAssignment, found := keeper.GetAssignment(ctx, roundStart, "inactive.example")
	require.True(t, found)
	inactiveAssignment.Finalized = true
	inactiveAssignment.FinalizedAtUnix = roundStart + 601
	require.NoError(t, keeper.SetAssignment(ctx, inactiveAssignment))

	require.NoError(t, keeper.settleFinalizedVerificationRounds(ctx))
	publisherPool, _, err := keeper.GetParams(ctx).RoundEmissionPools(3600)
	require.NoError(t, err)
	baseShare := publisherPool.QuoRaw(2)
	require.Equal(t, baseShare.MulRaw(1000).QuoRaw(10000), tokenomics.sent[ownerZero.String()])
	require.Equal(t, baseShare, tokenomics.sent[ownerFull.String()])
	_, inactivePaid := tokenomics.sent[ownerInactive.String()]
	require.False(t, inactivePaid)

	zeroAssignment, found := keeper.GetAssignment(ctx, roundStart, "zero.example")
	require.True(t, found)
	require.True(t, zeroAssignment.RewardsSettled)
	require.Empty(t, keeper.listUnsettledVerificationRounds(ctx))

	before := tokenomics.sent[ownerFull.String()]
	require.NoError(t, keeper.settleFinalizedVerificationRounds(ctx))
	require.Equal(t, before, tokenomics.sent[ownerFull.String()])
}
