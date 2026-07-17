package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"content-grid-chain/app"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/server"
	svrcmd "github.com/cosmos/cosmos-sdk/server/cmd"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	devnetEnvPrefix      = "CONTENT_GRID"
	defaultDevnetMoniker = "devnet-node"
	defaultDevnetKeyName = "validator"
	defaultDevnetBackend = "test"
	defaultDevnetBank    = "100000000ucongrid"
	defaultDevnetBond    = "1000000ucongrid"
)

const (
	flagDevnetMoniker    = "moniker"
	flagDevnetKeyName    = "key-name"
	flagDevnetBackend    = "keyring-backend"
	flagDevnetBankAmount = "amount"
	flagDevnetBondAmount = "bond-amount"
	flagDevnetForce      = "force"
)

func devnetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devnet",
		Short: "Bootstrap a single-validator network for local development",
		Long:  "devnet scaffolds a fresh home directory with a funded validator account, gentx, and collected genesis, so you can run 'start' immediately.",
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx := client.GetClientContextFromCmd(cmd)
			home := clientCtx.HomeDir
			if home == "" {
				home = app.DefaultNodeHome
			}
			if home == "" {
				return fmt.Errorf("unable to resolve node home directory")
			}

			serverCtx := server.GetServerContextFromCmd(cmd)
			cfg := serverCtx.Config
			cfg.SetRoot(home)

			moniker, err := cmd.Flags().GetString(flagDevnetMoniker)
			if err != nil {
				return err
			}
			keyName, err := cmd.Flags().GetString(flagDevnetKeyName)
			if err != nil {
				return err
			}
			backend, err := cmd.Flags().GetString(flagDevnetBackend)
			if err != nil {
				return err
			}
			bankAmount, err := cmd.Flags().GetString(flagDevnetBankAmount)
			if err != nil {
				return err
			}
			bondAmount, err := cmd.Flags().GetString(flagDevnetBondAmount)
			if err != nil {
				return err
			}
			force, err := cmd.Flags().GetBool(flagDevnetForce)
			if err != nil {
				return err
			}
			chainID, err := cmd.Flags().GetString(flags.FlagChainID)
			if err != nil {
				return err
			}
			if chainID == "" {
				chainID = fmt.Sprintf("grid-devnet-%d", time.Now().Unix())
			}

			allocationCoins, err := sdk.ParseCoinsNormalized(bankAmount)
			if err != nil {
				return fmt.Errorf("invalid devnet account amount: %w", err)
			}
			bondCoin, err := sdk.ParseCoinNormalized(bondAmount)
			if err != nil {
				return fmt.Errorf("invalid bond amount: %w", err)
			}
			if !allocationCoins.AmountOf(bondCoin.Denom).GTE(bondCoin.Amount) {
				return fmt.Errorf("bond amount %s exceeds provided account balance %s", bondCoin.String(), allocationCoins.String())
			}

			if force {
				if err := os.RemoveAll(home); err != nil {
					return fmt.Errorf("failed to clear home directory %s: %w", home, err)
				}
			} else {
				if _, err := os.Stat(cfg.GenesisFile()); err == nil {
					return fmt.Errorf("genesis file already exists at %s; rerun with --force to replace", cfg.GenesisFile())
				}
			}

			if err := os.MkdirAll(home, 0o755); err != nil {
				return fmt.Errorf("failed to create home directory %s: %w", home, err)
			}

			initArgs := []string{"init", moniker, "--chain-id", chainID}
			if force {
				initArgs = append(initArgs, "--overwrite")
			}
			if _, err := runDevnetStep(cmd, home, cmd.InOrStdin(), initArgs...); err != nil {
				return err
			}

			if err := patchGenesisDenoms(cfg.GenesisFile(), bondCoin.Denom); err != nil {
				return err
			}

			if _, err := runDevnetStep(cmd, home, cmd.InOrStdin(), "keys", "add", keyName, "--keyring-backend", backend); err != nil {
				return err
			}

			addrOutput, err := executeDevnetCommand(home, cmd.InOrStdin(), "keys", "show", keyName, "--keyring-backend", backend, "--address")
			if err != nil {
				fmt.Fprint(cmd.ErrOrStderr(), addrOutput)
				return fmt.Errorf("failed to fetch validator address: %w", err)
			}
			address := strings.TrimSpace(addrOutput)
			if address == "" {
				return fmt.Errorf("validator address is empty")
			}

			if _, err := runDevnetStep(cmd, home, cmd.InOrStdin(), "genesis", "add-genesis-account", address, bankAmount); err != nil {
				return err
			}

			gentxArgs := []string{"genesis", "gentx", keyName, bondAmount, "--chain-id", chainID, "--keyring-backend", backend}
			if _, err := runDevnetStep(cmd, home, cmd.InOrStdin(), gentxArgs...); err != nil {
				return err
			}

			if _, err := runDevnetStep(cmd, home, cmd.InOrStdin(), "genesis", "collect-gentxs"); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "\nLocal devnet is ready in %s\n", home)
			fmt.Fprintf(cmd.OutOrStdout(), "Validator key: %s (keyring backend: %s)\n", keyName, backend)
			fmt.Fprintf(cmd.OutOrStdout(), "Run '%s start --home %s' to launch the node.\n", cmd.Root().Name(), home)

			return nil
		},
	}

	cmd.Flags().String(flagDevnetMoniker, defaultDevnetMoniker, "moniker to use during init")
	cmd.Flags().String(flagDevnetKeyName, defaultDevnetKeyName, "validator key name to create")
	cmd.Flags().String(flagDevnetBackend, defaultDevnetBackend, "keyring backend for validator key")
	cmd.Flags().String(flagDevnetBankAmount, defaultDevnetBank, "total tokens to allocate to the validator genesis account (coin format)")
	cmd.Flags().String(flagDevnetBondAmount, defaultDevnetBond, "self delegation amount included in the gentx (coin format)")
	cmd.Flags().Bool(flagDevnetForce, false, "remove any existing data at --home before scaffolding")
	cmd.Flags().String(flags.FlagChainID, "", "explicit chain-id to set in the genesis (default: generated)")

	return cmd
}

func runDevnetStep(cmd *cobra.Command, home string, in io.Reader, args ...string) (string, error) {
	output, err := executeDevnetCommand(home, in, args...)
	if err != nil {
		if output != "" {
			fmt.Fprint(cmd.ErrOrStderr(), output)
		}
		return output, fmt.Errorf("command '%s' failed: %w", strings.Join(args, " "), err)
	}
	if output != "" {
		fmt.Fprint(cmd.OutOrStdout(), output)
	}
	return output, nil
}

func executeDevnetCommand(home string, in io.Reader, args ...string) (string, error) {
	root := NewRootCmd()
	root.SetIn(in)
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	fullArgs := append([]string{"--home", home}, args...)
	root.SetArgs(fullArgs)
	err := svrcmd.Execute(root, devnetEnvPrefix, resolveHomeArg(fullArgs, app.DefaultNodeHome))
	return buf.String(), err
}

func patchGenesisDenoms(genesisPath string, denom string) error {
	if strings.TrimSpace(genesisPath) == "" {
		return fmt.Errorf("genesis path required")
	}
	if strings.TrimSpace(denom) == "" {
		return fmt.Errorf("denom required")
	}
	bz, err := os.ReadFile(genesisPath)
	if err != nil {
		return fmt.Errorf("read genesis: %w", err)
	}
	var g map[string]any
	if err := json.Unmarshal(bz, &g); err != nil {
		return fmt.Errorf("decode genesis: %w", err)
	}
	appState, _ := g["app_state"].(map[string]any)
	if appState == nil {
		appState = map[string]any{}
		g["app_state"] = appState
	}

	if staking, ok := appState["staking"].(map[string]any); ok {
		if params, ok := staking["params"].(map[string]any); ok {
			params["bond_denom"] = denom
		}
	}
	if mint, ok := appState["mint"].(map[string]any); ok {
		if params, ok := mint["params"].(map[string]any); ok {
			params["mint_denom"] = denom
		}
	}
	if gov, ok := appState["gov"].(map[string]any); ok {
		if params, ok := gov["params"].(map[string]any); ok {
			for _, key := range []string{"min_deposit", "expedited_min_deposit"} {
				if v, ok := params[key].([]any); ok {
					for _, item := range v {
						if coin, ok := item.(map[string]any); ok {
							coin["denom"] = denom
						}
					}
				}
			}
		}
	}

	out, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("encode genesis: %w", err)
	}
	if err := os.WriteFile(genesisPath, out, 0o644); err != nil {
		return fmt.Errorf("write genesis: %w", err)
	}
	return nil
}
