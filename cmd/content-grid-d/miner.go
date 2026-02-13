package main

import (
	"fmt"

	typespb "content-grid-chain/x/miners/typespb"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
)

func minerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "miner",
		Short: "Miner management commands",
	}

	cmd.AddCommand(
		minerRegisterCmd(),
		minerUpdateCmd(),
		minerStakeCmd(),
	)

	return cmd
}

func minerRegisterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "register [metadata-uri] [services] [min-bid-amount]",
		Short: "Register as a miner",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			minBidDenom, err := cmd.Flags().GetString("min-bid-denom")
			if err != nil {
				return err
			}
			stake, err := cmd.Flags().GetString("stake")
			if err != nil {
				return err
			}

			msg := &typespb.MsgRegisterMiner{
				Operator:     clientCtx.GetFromAddress().String(),
				MetadataUri:  args[0],
				Services:     parseServiceMask(args[1]),
				MinBidDenom:  minBidDenom,
				MinBidAmount: args[2],
				Stake:        stake,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().String("min-bid-denom", "ucongrid", "Denomination for minimum bid")
	cmd.Flags().String("stake", "1000000", "Initial recorded stake amount (in ucongrid units)")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func minerUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update miner metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			metadata, err := cmd.Flags().GetString("metadata-uri")
			if err != nil {
				return err
			}
			servicesStr, err := cmd.Flags().GetString("services")
			if err != nil {
				return err
			}
			minBidAmount, err := cmd.Flags().GetString("min-bid-amount")
			if err != nil {
				return err
			}
			minBidDenom, err := cmd.Flags().GetString("min-bid-denom")
			if err != nil {
				return err
			}

			msg := &typespb.MsgUpdateMiner{
				Operator:     clientCtx.GetFromAddress().String(),
				MetadataUri:  metadata,
				Services:     parseServiceMask(servicesStr),
				MinBidAmount: minBidAmount,
				MinBidDenom:  minBidDenom,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String("metadata-uri", "", "New metadata URI")
	cmd.Flags().String("services", "", "Service bitmask (hex or decimal)")
	cmd.Flags().String("min-bid-amount", "", "New minimum bid amount")
	cmd.Flags().String("min-bid-denom", "", "New minimum bid denom")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func minerStakeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stake [amount]",
		Short: "Increase or decrease recorded stake",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			decrease, err := cmd.Flags().GetBool("decrease")
			if err != nil {
				return err
			}
			msg := &typespb.MsgUpdateMinerStake{
				Operator:   clientCtx.GetFromAddress().String(),
				StakeDelta: args[0],
				Increase:   !decrease,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().Bool("decrease", false, "Reduce stake instead of increasing")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func parseServiceMask(input string) uint32 {
	if input == "" {
		return 0
	}
	var mask uint32
	if _, err := fmt.Sscanf(input, "%d", &mask); err == nil {
		return mask
	}
	_, err := fmt.Sscanf(input, "0x%x", &mask)
	if err != nil {
		return 0
	}
	return mask
}
