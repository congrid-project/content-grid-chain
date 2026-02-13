package main

import (
	"fmt"
	"strings"
	"time"

	registry "content-grid-chain/x/registry"
	typespb "content-grid-chain/x/registry/typespb"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"
)

func registryTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Registry transactions (publishers, slots, leases)",
	}

	cmd.AddCommand(
		registryRegisterPublisherTxCmd(),
		registryCreateSlotTxCmd(),
		registryUpdateSlotStatusTxCmd(),
		registryLeaseSlotTxCmd(),
		registrySubmitDrandBeaconTxCmd(),
	)
	return cmd
}

func registryRegisterPublisherTxCmd() *cobra.Command {
	var metadataURI string
	var verifier string
	var referrer string

	cmd := &cobra.Command{
		Use:   "register-publisher [domain]",
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
	cmd.Flags().StringVar(&referrer, "referrer", "", "optional referrer address")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func registryCreateSlotTxCmd() *cobra.Command {
	var (
		domain             string
		label              string
		summary            string
		category           string
		placement          string
		size               string
		rateDenom          string
		rateAmount         string
		unitSeconds        int64
		minDurationSeconds int64
		maxDurationSeconds int64
		tags               []string
	)

	cmd := &cobra.Command{
		Use:   "create-slot",
		Short: "Create a link slot listing for a verified publisher",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			normDomain := registry.NormalizeDomain(domain)
			if normDomain == "" {
				return fmt.Errorf("--domain required")
			}
			if !registry.IsDomainFormatValid(normDomain) {
				return fmt.Errorf("domain %s is not a valid hostname", normDomain)
			}
			if strings.TrimSpace(label) == "" {
				return fmt.Errorf("--label required")
			}
			if strings.TrimSpace(rateDenom) == "" {
				return fmt.Errorf("--rate-denom required")
			}
			if strings.TrimSpace(rateAmount) == "" {
				return fmt.Errorf("--rate-amount required")
			}

			publisher := clientCtx.GetFromAddress().String()
			msg := &typespb.MsgCreateSlot{
				Publisher:          publisher,
				Domain:             normDomain,
				Label:              strings.TrimSpace(label),
				Summary:            strings.TrimSpace(summary),
				Category:           strings.TrimSpace(category),
				Placement:          strings.TrimSpace(placement),
				Size:               strings.TrimSpace(size),
				RateDenom:          strings.TrimSpace(rateDenom),
				RateAmount:         strings.TrimSpace(rateAmount),
				UnitSeconds:        unitSeconds,
				MinDurationSeconds: minDurationSeconds,
				MaxDurationSeconds: maxDurationSeconds,
				Tags:               tags,
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().StringVar(&domain, "domain", "", "publisher domain")
	cmd.Flags().StringVar(&label, "label", "", "slot label")
	cmd.Flags().StringVar(&summary, "summary", "", "slot summary")
	cmd.Flags().StringVar(&category, "category", "", "slot category")
	cmd.Flags().StringVar(&placement, "placement", "", "slot placement")
	cmd.Flags().StringVar(&size, "size", "", "slot size")
	cmd.Flags().StringVar(&rateDenom, "rate-denom", "ucongrid", "rate denom")
	cmd.Flags().StringVar(&rateAmount, "rate-amount", "", "rate amount (base units per unit)")
	cmd.Flags().Int64Var(&unitSeconds, "unit-seconds", int64((7 * time.Hour * 24).Seconds()), "billing unit in seconds")
	cmd.Flags().Int64Var(&minDurationSeconds, "min-duration-seconds", int64((7 * time.Hour * 24).Seconds()), "min lease duration in seconds")
	cmd.Flags().Int64Var(&maxDurationSeconds, "max-duration-seconds", int64((90 * time.Hour * 24).Seconds()), "max lease duration in seconds")
	cmd.Flags().StringArrayVar(&tags, "tags", nil, "slot tags (repeatable)")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func registryUpdateSlotStatusTxCmd() *cobra.Command {
	var slotID string
	var status string

	cmd := &cobra.Command{
		Use:   "update-slot-status",
		Short: "Update a slot listing status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			if strings.TrimSpace(slotID) == "" {
				return fmt.Errorf("--slot-id required")
			}
			if strings.TrimSpace(status) == "" {
				return fmt.Errorf("--status required")
			}

			st, ok := typespb.SlotStatus_value[strings.TrimSpace(status)]
			if !ok {
				return fmt.Errorf("invalid --status (use SLOT_STATUS_LISTED|SLOT_STATUS_PAUSED|SLOT_STATUS_UNLISTED)")
			}

			publisher := clientCtx.GetFromAddress().String()
			msg := &typespb.MsgUpdateSlotStatus{
				Publisher: publisher,
				SlotId:    strings.TrimSpace(slotID),
				Status:    typespb.SlotStatus(st),
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().StringVar(&slotID, "slot-id", "", "slot id")
	cmd.Flags().StringVar(&status, "status", "", "slot status enum name")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func registryLeaseSlotTxCmd() *cobra.Command {
	var slotID string
	var targetURL string
	var startsAtUnix int64
	var durationSeconds int64

	cmd := &cobra.Command{
		Use:   "lease-slot",
		Short: "Lease a slot (escrows payment on-chain)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			if strings.TrimSpace(slotID) == "" {
				return fmt.Errorf("--slot-id required")
			}
			if strings.TrimSpace(targetURL) == "" {
				return fmt.Errorf("--target-url required")
			}
			if durationSeconds <= 0 {
				return fmt.Errorf("--duration-seconds required")
			}
			if startsAtUnix == 0 {
				startsAtUnix = time.Now().UTC().Unix() + 5
			}

			lessee := clientCtx.GetFromAddress().String()
			msg := &typespb.MsgLeaseSlot{
				Lessee:          lessee,
				SlotId:          strings.TrimSpace(slotID),
				TargetUrl:       strings.TrimSpace(targetURL),
				StartsAtUnix:    startsAtUnix,
				DurationSeconds: durationSeconds,
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().StringVar(&slotID, "slot-id", "", "slot id")
	cmd.Flags().StringVar(&targetURL, "target-url", "", "lease target url")
	cmd.Flags().Int64Var(&startsAtUnix, "starts-at-unix", 0, "lease start time (unix seconds); defaults to now+5s")
	cmd.Flags().Int64Var(&durationSeconds, "duration-seconds", 0, "lease duration (seconds)")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func registrySubmitDrandBeaconTxCmd() *cobra.Command {
	var round uint64
	var randomnessHex string
	var signatureHex string

	cmd := &cobra.Command{
		Use:   "submit-drand-beacon",
		Short: "Submit a drand beacon for randomness mixing",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			if round == 0 {
				return fmt.Errorf("--round required")
			}
			if strings.TrimSpace(randomnessHex) == "" {
				return fmt.Errorf("--randomness-hex required")
			}
			msg := &typespb.MsgSubmitDrandBeacon{
				Submitter:     clientCtx.GetFromAddress().String(),
				Round:         round,
				RandomnessHex: strings.TrimSpace(strings.ToLower(randomnessHex)),
				SignatureHex:  strings.TrimSpace(strings.ToLower(signatureHex)),
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	cmd.Flags().Uint64Var(&round, "round", 0, "drand round")
	cmd.Flags().StringVar(&randomnessHex, "randomness-hex", "", "drand randomness hex")
	cmd.Flags().StringVar(&signatureHex, "signature-hex", "", "drand signature hex (optional, recommended)")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func registryQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Registry queries (publishers, slots, leases)",
	}
	cmd.AddCommand(
		registryQueryPublisherCmd(),
		registryQueryRoundMetaCmd(),
		registryQueryDrandBeaconCmd(),
		registryQueryLatestDrandBeaconCmd(),
		registryQuerySlotsCmd(),
		registryQueryLeasesCmd(),
	)
	return cmd
}

func registryQueryPublisherCmd() *cobra.Command {
	var domain string
	cmd := &cobra.Command{
		Use:   "publisher",
		Short: "Query publisher by domain",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			qctx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			if strings.TrimSpace(domain) == "" {
				return fmt.Errorf("--domain required")
			}
			queryClient := typespb.NewQueryClient(qctx)
			resp, err := queryClient.Publisher(cmd.Context(), &typespb.QueryPublisherRequest{Domain: registry.NormalizeDomain(domain)})
			if err != nil {
				return err
			}
			return qctx.PrintProto(resp)
		},
	}
	cmd.Flags().StringVar(&domain, "domain", "", "publisher domain")
	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}

func registryQueryRoundMetaCmd() *cobra.Command {
	var roundStartUnix int64
	cmd := &cobra.Command{
		Use:   "round-meta",
		Short: "Query deterministic metadata for a verification round",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			qctx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			if roundStartUnix <= 0 {
				return fmt.Errorf("--round-start-unix required")
			}
			queryClient := typespb.NewQueryClient(qctx)
			resp, err := queryClient.RoundMeta(cmd.Context(), &typespb.QueryRoundMetaRequest{RoundStartUnix: roundStartUnix})
			if err != nil {
				return err
			}
			return qctx.PrintProto(resp)
		},
	}
	cmd.Flags().Int64Var(&roundStartUnix, "round-start-unix", 0, "round start unix seconds")
	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}

func registryQueryDrandBeaconCmd() *cobra.Command {
	var round uint64
	cmd := &cobra.Command{
		Use:   "drand-beacon",
		Short: "Query drand beacon by round",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			qctx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			if round == 0 {
				return fmt.Errorf("--round required")
			}
			queryClient := typespb.NewQueryClient(qctx)
			resp, err := queryClient.DrandBeacon(cmd.Context(), &typespb.QueryDrandBeaconRequest{Round: round})
			if err != nil {
				return err
			}
			return qctx.PrintProto(resp)
		},
	}
	cmd.Flags().Uint64Var(&round, "round", 0, "drand round")
	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}

func registryQueryLatestDrandBeaconCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "latest-drand-beacon",
		Short: "Query latest drand beacon",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			qctx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			queryClient := typespb.NewQueryClient(qctx)
			resp, err := queryClient.LatestDrandBeacon(cmd.Context(), &typespb.QueryLatestDrandBeaconRequest{})
			if err != nil {
				return err
			}
			return qctx.PrintProto(resp)
		},
	}
	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}

func registryQuerySlotsCmd() *cobra.Command {
	var publisher string
	var status string
	cmd := &cobra.Command{
		Use:   "slots",
		Short: "List slots",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			qctx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			st := typespb.SlotStatus_SLOT_STATUS_UNSPECIFIED
			if strings.TrimSpace(status) != "" {
				val, ok := typespb.SlotStatus_value[strings.TrimSpace(status)]
				if !ok {
					return fmt.Errorf("invalid --status")
				}
				st = typespb.SlotStatus(val)
			}
			queryClient := typespb.NewQueryClient(qctx)
			resp, err := queryClient.Slots(cmd.Context(), &typespb.QuerySlotsRequest{Publisher: strings.TrimSpace(publisher), Status: st})
			if err != nil {
				return err
			}
			return qctx.PrintProto(resp)
		},
	}
	cmd.Flags().StringVar(&publisher, "publisher", "", "publisher bech32 address (optional)")
	cmd.Flags().StringVar(&status, "status", "", "slot status enum name (optional)")
	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}

func registryQueryLeasesCmd() *cobra.Command {
	var publisher string
	var slotID string
	var activeOnly bool
	cmd := &cobra.Command{
		Use:   "leases",
		Short: "List leases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			qctx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			queryClient := typespb.NewQueryClient(qctx)
			resp, err := queryClient.Leases(cmd.Context(), &typespb.QueryLeasesRequest{Publisher: strings.TrimSpace(publisher), SlotId: strings.TrimSpace(slotID), ActiveOnly: activeOnly, AtUnix: time.Now().UTC().Unix()})
			if err != nil {
				return err
			}
			return qctx.PrintProto(resp)
		},
	}
	cmd.Flags().StringVar(&publisher, "publisher", "", "publisher bech32 address (optional)")
	cmd.Flags().StringVar(&slotID, "slot-id", "", "slot id (optional)")
	cmd.Flags().BoolVar(&activeOnly, "active-only", false, "only include active leases")
	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}
