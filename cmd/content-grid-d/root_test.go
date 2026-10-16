package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cosmos/cosmos-sdk/client"
	svrcmd "github.com/cosmos/cosmos-sdk/server/cmd"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"content-grid-chain/internal/network"
)

func TestTxChainIDContext(t *testing.T) {
	for _, command := range [][]string{{"publisher", "register"}, {"verifier", "bond"}, {"tx", "bank", "send"}, {"tx", "ibc-transfer", "transfer"}} {
		for _, tc := range []struct {
			name, configured, explicit, want string
		}{
			{name: "default", want: network.DefaultChainID},
			{name: "configured", configured: "configured-chain", want: "configured-chain"},
			{name: "explicit", explicit: "custom-chain", want: "custom-chain"},
			{name: "explicit-over-config", configured: "configured-chain", explicit: "custom-chain", want: "custom-chain"},
		} {
			t.Run(command[0]+"/"+command[len(command)-1]+"/"+tc.name, func(t *testing.T) {
				home := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(home, "config"), 0o755))
				// Include an existing empty client config to exercise the legacy-home fallback.
				require.NoError(t, os.WriteFile(filepath.Join(home, "config", "client.toml"), []byte("chain-id = \""+tc.configured+"\"\n"), 0o600))
				root := NewRootCmd()
				cmd, _, err := root.Find(command)
				require.NoError(t, err)
				cmd.Args = nil
				var got string
				cmd.RunE = func(cmd *cobra.Command, _ []string) error {
					ctx, err := client.GetClientTxContext(cmd)
					got = ctx.ChainID
					return err
				}
				args := append([]string{"--home", home}, command...)
				if tc.explicit != "" {
					args = append(args, "--chain-id", tc.explicit)
				}
				root.SetArgs(args)
				require.NoError(t, svrcmd.Execute(root, "CONTENT_GRID", home))
				require.Equal(t, tc.want, got)
			})
		}
	}
}

func TestUnexpectedRelativeAppDBAllowsDatabaseInsideHome(t *testing.T) {
	home := t.TempDir()
	dbPath := filepath.Join(home, "data", "application.db")
	if err := os.MkdirAll(dbPath, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := unexpectedRelativeAppDB(home, home); got != "" {
		t.Fatalf("expected database inside home to be allowed, got %q", got)
	}
}

func TestUnexpectedRelativeAppDBRejectsDatabaseOutsideHome(t *testing.T) {
	home := t.TempDir()
	cwd := t.TempDir()
	dbPath := filepath.Join(cwd, "data", "application.db")
	if err := os.MkdirAll(dbPath, 0o755); err != nil {
		t.Fatal(err)
	}

	if got := unexpectedRelativeAppDB(cwd, home); got != dbPath {
		t.Fatalf("expected %q, got %q", dbPath, got)
	}
}

func TestUnexpectedRelativeAppDBIgnoresMissingDatabase(t *testing.T) {
	if got := unexpectedRelativeAppDB(t.TempDir(), t.TempDir()); got != "" {
		t.Fatalf("expected missing database to be ignored, got %q", got)
	}
}
