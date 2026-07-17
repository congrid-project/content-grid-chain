package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
