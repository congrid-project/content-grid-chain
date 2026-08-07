package app

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/stretchr/testify/require"

	"content-grid-chain/x/nodes"
	"content-grid-chain/x/registry"
	"content-grid-chain/x/tokenomics"
	"content-grid-chain/x/verifiers"
)

func TestPrepareDrandStrictV2VersionMap(t *testing.T) {
	fromVM := module.VersionMap{"bank": 4}
	targetVM := module.VersionMap{
		nodes.ModuleName:      1,
		registry.ModuleName:   2,
		verifiers.ModuleName:  1,
		tokenomics.ModuleName: 1,
	}

	got := prepareDrandStrictV2VersionMap(fromVM, targetVM)
	require.Equal(t, uint64(4), got["bank"])
	require.Equal(t, uint64(1), got[nodes.ModuleName])
	require.Equal(t, uint64(1), got[registry.ModuleName], "missing registry must migrate from its live-chain v1 state")
	require.Equal(t, uint64(1), got[verifiers.ModuleName])
	require.Equal(t, uint64(1), got[tokenomics.ModuleName])
	require.NotContains(t, fromVM, registry.ModuleName, "input version map must not be mutated")
}

func TestPrepareDrandStrictV2VersionMapPreservesRecordedVersions(t *testing.T) {
	fromVM := module.VersionMap{
		registry.ModuleName:   1,
		verifiers.ModuleName:  7,
		tokenomics.ModuleName: 8,
		nodes.ModuleName:      9,
	}
	targetVM := module.VersionMap{
		registry.ModuleName:   2,
		verifiers.ModuleName:  1,
		tokenomics.ModuleName: 1,
		nodes.ModuleName:      1,
	}

	got := prepareDrandStrictV2VersionMap(fromVM, targetVM)
	require.Equal(t, fromVM, got)
}

func TestPreparePublisherRewardsV3VersionMap(t *testing.T) {
	fromVM := module.VersionMap{"bank": 4}
	targetVM := module.VersionMap{
		nodes.ModuleName:      1,
		registry.ModuleName:   3,
		verifiers.ModuleName:  1,
		tokenomics.ModuleName: 1,
	}

	got := preparePublisherRewardsV3VersionMap(fromVM, targetVM)
	require.Equal(t, uint64(4), got["bank"])
	require.Equal(t, uint64(2), got[registry.ModuleName])
	require.Equal(t, uint64(1), got[nodes.ModuleName])
	require.Equal(t, uint64(1), got[verifiers.ModuleName])
	require.Equal(t, uint64(1), got[tokenomics.ModuleName])
	require.NotContains(t, fromVM, registry.ModuleName)
}
