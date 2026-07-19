package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFetchDrandRoundRequestsExactRound(t *testing.T) {
	const chainHash = "52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/"+chainHash+"/public/42", r.URL.Path)
		_, _ = fmt.Fprint(w, `{"round":42,"randomness":"abcd","signature":"ef01"}`)
	}))
	defer server.Close()

	agent := &Agent{
		Cfg:        Config{Drand: DrandRelayConfig{APIBaseURL: server.URL, RequestTimeoutSec: 2}},
		HTTPClient: server.Client(),
	}
	beacon, err := agent.fetchDrandRound(context.Background(), chainHash, 42)
	require.NoError(t, err)
	require.Equal(t, uint64(42), beacon.Round)
}

func TestDrandRelayRankIsDeterministicAndUnique(t *testing.T) {
	active := []drandRelayCandidate{
		{Address: "congrid1carol", BondAmount: "50"},
		{Address: "congrid1alice", BondAmount: "50"},
		{Address: "congrid1bob", BondAmount: "50"},
		{Address: "congrid1alice", BondAmount: "50"},
	}
	ranks := make([]int, 0, 3)
	for _, verifier := range []string{"congrid1alice", "congrid1bob", "congrid1carol"} {
		rank, total, err := drandRelayRank(42, verifier, active)
		require.NoError(t, err)
		require.Equal(t, 3, total)
		rankAgain, _, err := drandRelayRank(42, verifier, []drandRelayCandidate{
			{Address: "congrid1bob", BondAmount: "50"},
			{Address: "congrid1carol", BondAmount: "50"},
			{Address: "congrid1alice", BondAmount: "50"},
		})
		require.NoError(t, err)
		require.Equal(t, rank, rankAgain)
		ranks = append(ranks, rank)
	}
	sort.Ints(ranks)
	require.Equal(t, []int{0, 1, 2}, ranks)
}

func TestDrandRelayRankRejectsInactiveVerifier(t *testing.T) {
	_, _, err := drandRelayRank(42, "congrid1inactive", []drandRelayCandidate{{Address: "congrid1active", BondAmount: "50"}})
	require.ErrorContains(t, err, "is not active")
}

func TestDrandRelayRankWeightsPrimaryByBond(t *testing.T) {
	active := []drandRelayCandidate{
		{Address: "congrid1large", BondAmount: "100"},
		{Address: "congrid1small", BondAmount: "1"},
	}
	largePrimary := 0
	for round := uint64(1); round <= 200; round++ {
		rank, _, err := drandRelayRank(round, "congrid1large", active)
		require.NoError(t, err)
		if rank == 0 {
			largePrimary++
		}
	}
	require.Greater(t, largePrimary, 180)
}

func TestDrandRelayDelayCapsDegradedFallbackRace(t *testing.T) {
	require.EqualValues(t, 0, drandRelayDelaySeconds(0, 60, 180))
	require.EqualValues(t, 60, drandRelayDelaySeconds(1, 60, 180))
	require.EqualValues(t, 180, drandRelayDelaySeconds(3, 60, 180))
	require.EqualValues(t, 180, drandRelayDelaySeconds(20, 60, 180))
}

func TestConfigDefaultsToMinimumGasPriceAndRelayStagger(t *testing.T) {
	cfg := Config{}
	cfg.applyDefaults()
	require.Equal(t, "250000", cfg.Submit.Gas)
	require.Empty(t, cfg.Submit.Fees)
	require.Equal(t, "0.001ucongrid", cfg.Submit.GasPrices)
	require.EqualValues(t, 60, cfg.Drand.RelayStaggerSec)
	require.EqualValues(t, 180, cfg.Drand.RelayMaxDelaySec)
	args := (&Agent{Cfg: cfg}).appendSubmitFlags([]string{"tx"})
	require.Contains(t, args, "--gas-prices")
	require.Contains(t, args, "0.001ucongrid")
	require.NotContains(t, args, "--fees")
}

func TestConfigRejectsFeesAndGasPricesTogether(t *testing.T) {
	cfg := Config{
		VerifierAddress: "congrid1verifier",
		Drand:           DrandRelayConfig{Disabled: true},
		Submit: SubmitConfig{
			ChainID:   "congrid-main",
			From:      "verifier-key",
			Fees:      "5000ucongrid",
			GasPrices: "0.001ucongrid",
		},
	}
	cfg.applyDefaults()
	err := cfg.Validate()
	require.ErrorContains(t, err, "mutually exclusive")
}

func TestAppendSubmitFlagsIncludesFeeGranter(t *testing.T) {
	agent := &Agent{Cfg: Config{Submit: SubmitConfig{
		Gas:        "250000",
		Fees:       "5000ucongrid",
		FeeGranter: "congrid1sponsor",
		Yes:        true,
	}}}
	args := agent.appendSubmitFlags([]string{"tx"})
	require.Contains(t, args, "--fee-granter")
	require.Contains(t, args, "congrid1sponsor")
	require.Contains(t, args, "--yes")
}
