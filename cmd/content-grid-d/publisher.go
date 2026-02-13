package main

import (
	"fmt"
	"strings"

	registryoffchain "content-grid-chain/offchain/registry"
	"content-grid-chain/x/registry"
	typespb "content-grid-chain/x/registry/typespb"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
)

func publisherCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "publisher",
		Short: "Publisher management helpers",
	}

	cmd.AddCommand(publisherRegisterCommand())
	return cmd
}

func publisherRegisterCommand() *cobra.Command {
	var metadataURI string
	var verifier string
	var referrer string

	cmd := &cobra.Command{
		Use:   "register [domain]",
		Short: "Register a publisher domain on-chain",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			domain := registry.NormalizeDomain(args[0])
			if domain == "" {
				return fmt.Errorf("domain required")
			}
			if !registry.IsDomainFormatValid(domain) {
				return fmt.Errorf("domain %s is not a valid hostname", domain)
			}

			owner := clientCtx.GetFromAddress().String()
			verifierClient := registryoffchain.HTTPContentVerifier{}
			if err := verifierClient.Verify(cmd.Context(), domain, owner); err != nil {
				return err
			}

			msg := &typespb.MsgRegisterPublisher{
				Owner:       owner,
				Domain:      domain,
				MetadataUri: strings.TrimSpace(metadataURI),
				Verifier:    strings.TrimSpace(verifier),
				Referrer:    strings.TrimSpace(referrer),
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().StringVar(&metadataURI, "metadata-uri", "", "optional metadata URI describing the publisher")
	cmd.Flags().StringVar(&verifier, "verifier", "", "optional verifier address attesting to this registration")
	cmd.Flags().StringVar(&referrer, "referrer", "", "optional referrer address (verifier referral incentive)")

	// NOTE: We intentionally do NOT support a /.well-known/content-grid.txt proof flow.
	// Publisher ownership verification is based solely on the homepage containing at least one congrid.net link.
	cmd.Flags().String(flags.FlagFrom, "", "Name or address of private key with which to sign")
	cmd.Flags().String(flags.FlagChainID, "", "The network chain ID")
	cmd.Flags().String(flags.FlagNode, "tcp://localhost:26657", "<host>:<port> to CometBFT rpc interface for this chain")
	cmd.Flags().String(flags.FlagKeyringBackend, "os", "Select keyring's backend (os|file|kwallet|pass|test|memory)")
	cmd.Flags().String(flags.FlagKeyringDir, "", "The client Keyring directory; if omitted, the default 'home' directory will be used")
	cmd.Flags().String(flags.FlagGas, "200000", "gas limit to set per-transaction; set to 'auto' to calculate sufficient gas automatically")
	cmd.Flags().Float64(flags.FlagGasAdjustment, 1, "adjustment factor to be multiplied against the estimate returned by the tx simulation")
	cmd.Flags().String(flags.FlagFees, "", "Fees to pay along with transaction; eg: 10uatom")
	cmd.Flags().String(flags.FlagGasPrices, "", "Gas prices in decimal format to determine the transaction fee (e.g. 0.1uatom)")
	cmd.Flags().String(flags.FlagBroadcastMode, "sync", "Transaction broadcasting mode (sync|async)")
	cmd.Flags().BoolP(flags.FlagSkipConfirmation, "y", false, "Skip tx broadcasting prompt confirmation")
	cmd.Flags().StringP(flags.FlagOutput, "o", "json", "Output format (text|json)")

	return cmd
}
