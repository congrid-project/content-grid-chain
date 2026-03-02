package registry

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func TestNormalizeAndValidateDomain(t *testing.T) {
	in := "  ExAmPlE.COM  "
	norm := NormalizeDomain(in)
	require.Equal(t, "example.com", norm)

	owner := sdk.AccAddress([]byte("publisher-owner-address")).String()
	w := Website{Domain: norm, Owner: owner, Status: StatusPending}
	require.NoError(t, ValidateWebsite(w))
}

func TestDomainReWithPort(t *testing.T) {
	require.True(t, IsDomainFormatValid("example.com"))
	require.True(t, IsDomainFormatValid("example.com:80"))
	require.True(t, IsDomainFormatValid("sub.example.com:8080"))
	require.False(t, IsDomainFormatValid("example.com:abc"))
	require.False(t, IsDomainFormatValid("invalid-domain"))
}

func TestGetPrimaryDomain(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"example.com", "example.com", false},
		{"sub.example.com", "example.com", false},
		{"api.v2.example.com:8080", "example.com", false},
		{"example.co.uk", "co.uk", false}, // Per simplified logic requirements
		{"localhost", "", true},           // Not a valid domain format per domainRe
		{"example", "", true},             // Not a valid domain format
	}

	for _, tt := range tests {
		got, err := GetPrimaryDomain(tt.in)
		if tt.wantErr {
			require.Error(t, err, tt.in)
		} else {
			require.NoError(t, err, tt.in)
			require.Equal(t, tt.want, got, tt.in)
		}
	}
}

func TestGenesisValidate(t *testing.T) {
	gs := DefaultGenesis()
	require.NoError(t, gs.Validate())

	bad := GenesisState{
		Websites: []Website{{Domain: "BAD_domain", Owner: sdk.AccAddress([]byte("publisher-owner-address")).String(), Status: StatusPending}},
		Params:   DefaultPublisherParams(),
	}
	require.Error(t, bad.Validate())
}

func TestPublisherScore(t *testing.T) {
	weights := DefaultPublisherScoreWeights()
	snap := PublisherSnapshot{
		Domain:             "example.com",
		OnlineMinutes:      690,
		ExpectedMinutes:    720,
		ReferralClicks:     1200,
		BaselineClicks:     400,
		LastVerifiedHeight: 1_000,
		CurrentHeight:      1_500,
		VerificationTTL:    2_000,
	}

	score, err := snap.Score(weights)
	require.NoError(t, err)
	require.True(t, score.GT(sdkmath.LegacyNewDecWithPrec(6, 1)))

	expired := snap
	expired.CurrentHeight = snap.LastVerifiedHeight + snap.VerificationTTL + 1
	expiredScore, err := expired.Score(weights)
	require.NoError(t, err)
	require.True(t, expiredScore.LT(score))
}

func TestPublisherParamsValidate(t *testing.T) {
	params := DefaultPublisherParams()
	require.NoError(t, params.Validate())

	params.MinVerifierCount = 0
	require.Error(t, params.Validate())

	params = DefaultPublisherParams()
	params.CommitWindowSeconds = 0
	require.Error(t, params.Validate())

	params = DefaultPublisherParams()
	params.CommitWindowSeconds = params.VerificationTTL
	require.Error(t, params.Validate())

	params = DefaultPublisherParams()
	params.CommitWindowSeconds = params.SubmissionWindowSeconds
	require.Error(t, params.Validate())

	params = DefaultPublisherParams()
	params.RequiredExternalLinksForFullReward = -1
	require.Error(t, params.Validate())

	params = DefaultPublisherParams()
	params.VerifierRewardBaseShareBps = -1
	require.Error(t, params.Validate())

	params = DefaultPublisherParams()
	params.VerifierRewardBaseShareBps = 10001
	require.Error(t, params.Validate())
}

func TestRoundEmissionPools(t *testing.T) {
	params := DefaultPublisherParams()
	publisher, verifier, err := params.RoundEmissionPools(3600)
	require.NoError(t, err)
	require.Equal(t, sdkmath.NewInt(114_155_251), publisher)
	require.Equal(t, sdkmath.NewInt(570_776_255), verifier)
}

func TestSplitPublisherRewards(t *testing.T) {
	params := DefaultPublisherParams()
	total := sdkmath.LegacyMustNewDecFromStr("1000")
	snapshot := PublisherSnapshot{
		Domain:               "example.com",
		VerifiedCongridLinks: 10,
		OnlineMinutes:        700,
		ExpectedMinutes:      720,
		ReferralClicks:       1100,
		BaselineClicks:       400,
		LastVerifiedHeight:   1_000,
		CurrentHeight:        1_500,
		VerificationTTL:      params.VerificationTTL,
	}
	reports := []VerifierReport{
		{
			Worker:            "congrid1verifiera",
			Checks:            50,
			Matches:           48,
			ExpectedChecks:    50,
			MedianLatencyMs:   4_000,
			TargetLatencyMs:   5_000,
			SubmittedAtHeight: 1_480,
		},
		{
			Worker:            "congrid1verifierb",
			Checks:            45,
			Matches:           40,
			ExpectedChecks:    50,
			MedianLatencyMs:   5_500,
			TargetLatencyMs:   5_000,
			SubmittedAtHeight: 1_495,
		},
		{
			Worker:            "congrid1verifierc",
			Checks:            30,
			Matches:           27,
			ExpectedChecks:    40,
			MedianLatencyMs:   4_800,
			TargetLatencyMs:   5_000,
			SubmittedAtHeight: 1_498,
		},
	}

	outcome, err := SplitPublisherRewards(total, snapshot, reports, params)
	require.NoError(t, err)
	require.True(t, outcome.PublisherAmount.Equal(total.Mul(params.RewardSplit.PublisherShare)))
	require.True(t, outcome.ProtocolAmount.Equal(total.Mul(params.RewardSplit.ProtocolShare)))
	verifierTotal := sdkmath.LegacyZeroDec()
	for _, amt := range outcome.VerifierAmounts {
		verifierTotal = verifierTotal.Add(amt)
	}
	require.True(t, verifierTotal.GT(sdkmath.LegacyZeroDec()))
	require.InDelta(t, total.Mul(params.RewardSplit.VerifierShare).MustFloat64(), verifierTotal.MustFloat64(), 1e-6)
	require.True(t, outcome.RolloverAmount.Abs().LTE(mustNewDec("0.0000001")))
}

func TestSplitPublisherRewardsZeroCongridLinks(t *testing.T) {
	params := DefaultPublisherParams()
	total := sdkmath.LegacyMustNewDecFromStr("500")
	snapshot := PublisherSnapshot{
		VerificationTTL:      params.VerificationTTL,
		VerifiedCongridLinks: 0,
	}
	reports := []VerifierReport{{Worker: "congrid1verifier", Checks: 10, Matches: 10, ExpectedChecks: 10, MedianLatencyMs: 4_000, TargetLatencyMs: 5_000, SubmittedAtHeight: 5}}

	outcome, err := SplitPublisherRewards(total, snapshot, reports, params)
	require.NoError(t, err)
	require.InDelta(t, total.MustFloat64(), outcome.RolloverAmount.MustFloat64(), 1e-6)
	require.True(t, outcome.PublisherAmount.IsZero())
	require.True(t, outcome.ProtocolAmount.IsZero())
	require.Zero(t, len(outcome.VerifierAmounts))
}

func TestSplitPublisherRewardsOneCongridLink(t *testing.T) {
	params := DefaultPublisherParams()
	total := sdkmath.LegacyMustNewDecFromStr("800")
	snapshot := PublisherSnapshot{
		Domain:               "example.com",
		VerifiedCongridLinks: 1,
		OnlineMinutes:        700,
		ExpectedMinutes:      720,
		ReferralClicks:       1100,
		BaselineClicks:       400,
		LastVerifiedHeight:   1_000,
		CurrentHeight:        1_500,
		VerificationTTL:      params.VerificationTTL,
	}
	reports := []VerifierReport{
		{
			Worker:            "congrid1verifiera",
			Checks:            50,
			Matches:           48,
			ExpectedChecks:    50,
			MedianLatencyMs:   4_000,
			TargetLatencyMs:   5_000,
			SubmittedAtHeight: 1_480,
		},
		{
			Worker:            "congrid1verifierb",
			Checks:            45,
			Matches:           40,
			ExpectedChecks:    50,
			MedianLatencyMs:   5_500,
			TargetLatencyMs:   5_000,
			SubmittedAtHeight: 1_495,
		},
	}

	outcome, err := SplitPublisherRewards(total, snapshot, reports, params)
	require.NoError(t, err)

	eligibleTotal := total.Mul(mustNewDec("0.10"))
	verifierTotal := sdkmath.LegacyZeroDec()
	for _, amt := range outcome.VerifierAmounts {
		verifierTotal = verifierTotal.Add(amt)
	}
	allocatedTotal := outcome.PublisherAmount.Add(outcome.ProtocolAmount).Add(verifierTotal)

	require.InDelta(t, eligibleTotal.MustFloat64(), allocatedTotal.MustFloat64(), 1e-6)
	require.InDelta(t, total.Sub(eligibleTotal).MustFloat64(), outcome.RolloverAmount.MustFloat64(), 1e-6)
}
