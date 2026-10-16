package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"content-grid-chain/internal/network"
)

func TestLoadConfigChainID(t *testing.T) {
	for _, tc := range []struct {
		name, submit, want string
	}{
		{name: "default", submit: `{"from":"verifier-key"}`, want: network.DefaultChainID},
		{name: "explicit", submit: `{"from":"verifier-key","chain_id":" custom-chain "}`, want: "custom-chain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			require.NoError(t, os.WriteFile(path, []byte(`{"verifier_address":"congrid1verifier","submit":`+tc.submit+`}`), 0o600))
			cfg, err := loadConfig(path)
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.Submit.ChainID)
			require.NoError(t, cfg.Validate())
		})
	}
}
