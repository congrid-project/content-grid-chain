package registry

import (
	"fmt"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"
)

func TestSelectDeterministicWeighted_DeterministicAndUnique(t *testing.T) {
	candidates := []string{"congrid1a", "congrid1b", "congrid1c", "congrid1d", "congrid1e"}
	weights := map[string]sdkmath.Int{
		"congrid1a": sdkmath.NewInt(5),
		"congrid1b": sdkmath.NewInt(10),
		"congrid1c": sdkmath.NewInt(15),
		"congrid1d": sdkmath.NewInt(20),
		"congrid1e": sdkmath.NewInt(25),
	}
	seed := []byte("round-seed-example")

	first := selectDeterministicWeighted(candidates, weights, 3, seed)
	second := selectDeterministicWeighted(candidates, weights, 3, seed)

	require.Equal(t, first, second)
	require.Len(t, first, 3)

	seen := map[string]struct{}{}
	for _, addr := range first {
		_, ok := weights[addr]
		require.True(t, ok, "selected unknown candidate %s", addr)
		_, dup := seen[addr]
		require.False(t, dup, "duplicate selection %s", addr)
		seen[addr] = struct{}{}
	}
}

func TestSelectDeterministicWeighted_PrefersHigherStake(t *testing.T) {
	candidates := []string{"congrid1smalla", "congrid1smallb", "congrid1whale"}
	weights := map[string]sdkmath.Int{
		"congrid1smalla": sdkmath.NewInt(1),
		"congrid1smallb": sdkmath.NewInt(1),
		"congrid1whale":  sdkmath.NewInt(1000),
	}

	hits := map[string]int{}
	for i := 0; i < 300; i++ {
		seed := []byte(fmt.Sprintf("seed-%d", i))
		picked := selectDeterministicWeighted(candidates, weights, 1, seed)
		require.Len(t, picked, 1)
		hits[picked[0]]++
	}

	// With 1000 stake vs 1+1, expected hit rate is ~99.8% for whale.
	// Use a loose lower bound to avoid test fragility while still proving weighting behavior.
	require.GreaterOrEqual(t, hits["congrid1whale"], 280)
}

func TestSplitVerifierAssignmentRewards_BasePlusWeighted(t *testing.T) {
	successful := []string{"congrid1a", "congrid1b", "congrid1c"}
	weights := map[string]sdkmath.Int{
		"congrid1a": sdkmath.NewInt(10),
		"congrid1b": sdkmath.NewInt(30),
		"congrid1c": sdkmath.NewInt(60),
	}

	payouts, remaining := splitVerifierAssignmentRewards(sdkmath.NewInt(1000), successful, weights, 4000)
	require.True(t, remaining.IsZero())
	require.Equal(t, sdkmath.NewInt(193), payouts["congrid1a"])
	require.Equal(t, sdkmath.NewInt(313), payouts["congrid1b"])
	require.Equal(t, sdkmath.NewInt(494), payouts["congrid1c"])
}

func TestSplitVerifierAssignmentRewards_WeightedBurnWhenNoWeights(t *testing.T) {
	successful := []string{"congrid1a", "congrid1b"}
	weights := map[string]sdkmath.Int{}

	payouts, remaining := splitVerifierAssignmentRewards(sdkmath.NewInt(1000), successful, weights, 4000)
	require.Equal(t, sdkmath.NewInt(200), payouts["congrid1a"])
	require.Equal(t, sdkmath.NewInt(200), payouts["congrid1b"])
	require.Equal(t, sdkmath.NewInt(600), remaining)
}
