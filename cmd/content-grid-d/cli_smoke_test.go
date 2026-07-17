package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"content-grid-chain/app"

	svrcmd "github.com/cosmos/cosmos-sdk/server/cmd"
	"github.com/stretchr/testify/require"
)

// TestCLISmokeLifecycle exercises the minimal CLI flow used in the quick-start docs.
func TestCLISmokeLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI smoke test in short mode")
	}

	homeDir := t.TempDir()
	chainID := "grid-cli-smoke"

	runCLI(t, homeDir, "init", "smoke-node", "--chain-id", chainID)

	configPath := filepath.Join(homeDir, "config", "config.toml")
	configContents := string(readFile(t, configPath))
	require.Contains(t, configContents, `timeout_commit = "30s"`)

	runCLI(t, homeDir, "keys", "add", "validator", "--keyring-backend", "test")
	addr := strings.TrimSpace(runCLI(t, homeDir, "keys", "show", "validator", "--keyring-backend", "test", "--address"))
	require.NotEmpty(t, addr)

	runCLI(t, homeDir, "genesis", "add-genesis-account", addr, "100000000ucongrid")
	runCLI(t, homeDir, "genesis", "gentx", "validator", "1000000ucongrid", "--chain-id", chainID, "--keyring-backend", "test")
	runCLI(t, homeDir, "genesis", "collect-gentxs")

	genesisPath := filepath.Join(homeDir, "config", "genesis.json")
	contents := readFile(t, genesisPath)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(contents, &doc))
	appState, ok := doc["app_state"].(map[string]any)
	require.True(t, ok, "missing app_state")
	genutil, ok := appState["genutil"].(map[string]any)
	require.True(t, ok, "missing genutil state")
	genTxs, ok := genutil["gen_txs"].([]any)
	require.True(t, ok, "gen_txs should be an array")
	require.NotEmpty(t, genTxs, "expected collected gentx in genesis")
}

func TestVersionCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI smoke test in short mode")
	}

	homeDir := t.TempDir()
	output := strings.TrimSpace(runCLI(t, homeDir, "version"))
	require.NotEmpty(t, output, "expected version output")
}

func TestDevnetCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI smoke test in short mode")
	}

	homeDir := t.TempDir()
	chainID := "grid-devnet-smoke"

	runCLI(t, homeDir, "devnet", "--moniker", "devnet-node", "--chain-id", chainID, "--keyring-backend", "test")

	genesisPath := filepath.Join(homeDir, "config", "genesis.json")
	contents := readFile(t, genesisPath)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(contents, &doc))
	appState, ok := doc["app_state"].(map[string]any)
	require.True(t, ok, "missing app_state")
	genutil, ok := appState["genutil"].(map[string]any)
	require.True(t, ok, "missing genutil state")
	genTxs, ok := genutil["gen_txs"].([]any)
	require.True(t, ok, "gen_txs should be an array")
	require.NotEmpty(t, genTxs, "expected collected gentx in genesis")
	bankState, ok := appState["bank"].(map[string]any)
	require.True(t, ok, "missing bank state")
	balances, ok := bankState["balances"].([]any)
	require.True(t, ok, "balances should be an array")
	require.NotEmpty(t, balances, "expected funded validator balance")
	authState, ok := appState["auth"].(map[string]any)
	require.True(t, ok, "missing auth state")
	accounts, ok := authState["accounts"].([]any)
	require.True(t, ok, "accounts should be an array")
	require.NotEmpty(t, accounts, "expected validator account in genesis")
}

func TestResolveHomeArg(t *testing.T) {
	require.Equal(t, "/custom/home", resolveHomeArg([]string{"--home", "/custom/home", "version"}, "/default/home"))
	require.Equal(t, "/custom/home", resolveHomeArg([]string{"version", "--home=/custom/home"}, "/default/home"))
	require.Equal(t, "/default/home", resolveHomeArg([]string{"version"}, "/default/home"))
}

func TestExplicitHomeDoesNotCreateDefaultHome(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping CLI smoke test in short mode")
	}

	baseDir := t.TempDir()
	defaultHome := filepath.Join(baseDir, "default-home")
	explicitHome := filepath.Join(baseDir, "explicit-home")

	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	args := []string{"--home", explicitHome, "init", "home-priority-node", "--chain-id", "home-priority-test"}
	cmd.SetArgs(args)

	require.NoError(t, svrcmd.Execute(cmd, "CONTENT_GRID", resolveHomeArg(args, defaultHome)), "command failed: %v", args)
	require.DirExists(t, explicitHome)
	require.NoDirExists(t, defaultHome)
}

func readFile(t *testing.T, path string) []byte {
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func runCLI(t *testing.T, homeDir string, args ...string) string {
	cmd := NewRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	fullArgs := append([]string{"--home", homeDir}, args...)
	cmd.SetArgs(fullArgs)
	require.NoError(t, svrcmd.Execute(cmd, "CONTENT_GRID", resolveHomeArg(fullArgs, app.DefaultNodeHome)), "command failed: %v", fullArgs)
	return buf.String()
}
