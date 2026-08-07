package main

import (
	"fmt"
	"strings"
	"time"

	"content-grid-chain/x/registry"
	registrypb "content-grid-chain/x/registry/typespb"
	typespb "content-grid-chain/x/verifiers/typespb"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"
)

func verifierCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verifier",
		Short: "Verifier bonding commands",
	}

	cmd.AddCommand(
		verifierBondCmd(),
		verifierUnbondCmd(),
		verifierCommitCmd(),
		verifierRevealCmd(),
		verifierAssignmentsCmd(),
	)

	return cmd
}

func verifierBondCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bond [amount]",
		Short: "Bond tokens into escrow to become an eligible verifier",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			denom, err := cmd.Flags().GetString("denom")
			if err != nil {
				return err
			}
			msg := &typespb.MsgBond{Verifier: clientCtx.GetFromAddress().String(), Denom: denom, Amount: args[0]}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String("denom", "ucongrid", "bond denom")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func verifierUnbondCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unbond [amount]",
		Short: "Unbond tokens from escrow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			denom, err := cmd.Flags().GetString("denom")
			if err != nil {
				return err
			}
			msg := &typespb.MsgUnbond{Verifier: clientCtx.GetFromAddress().String(), Denom: denom, Amount: args[0]}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().String("denom", "ucongrid", "bond denom")
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

var _ = fmt.Errorf

type similarEvidenceFlags struct {
	observed        int32
	matched         int32
	expected        int32
	expectedSetHash string
	observedSetHash string
}

func (f *similarEvidenceFlags) addTo(cmd *cobra.Command) {
	cmd.Flags().Int32Var(&f.observed, "observed-similar-domains", 0, "unique similar domains observed in the publisher page")
	cmd.Flags().Int32Var(&f.matched, "matched-similar-domains", 0, "observed domains matching the expected set")
	cmd.Flags().Int32Var(&f.expected, "expected-similar-domains", 0, "domains returned by the expected similar set")
	cmd.Flags().StringVar(&f.expectedSetHash, "expected-set-hash", "", "hash of the expected similar-domain set")
	cmd.Flags().StringVar(&f.observedSetHash, "observed-set-hash", "", "hash of the observed similar-domain set")
}

func (f similarEvidenceFlags) resolveHash(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw != "" || !registrypb.HasVerificationEvidence(f.observed, f.matched, f.expected, f.expectedSetHash, f.observedSetHash) {
		return raw
	}
	return registrypb.ComputeVerificationEvidenceHash(f.observed, f.matched, f.expected, f.expectedSetHash, f.observedSetHash)
}

func verifierCommitCmd() *cobra.Command {
	var (
		passedFlag   bool
		failedFlag   bool
		roundStart   int64
		includeFinal bool

		commitHash        string
		evidenceHash      string
		nonce             string
		verificationOwner string
		similar           similarEvidenceFlags
	)
	cmd := &cobra.Command{
		Use:   "commit [domain]",
		Short: "Submit a verification commit hash for an assigned publisher",
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

			if roundStart == 0 {
				qctx, err := client.GetClientQueryContext(cmd)
				if err != nil {
					return err
				}
				queryClient := registrypb.NewQueryClient(qctx)
				resp, err := queryClient.VerifierAssignments(cmd.Context(), &registrypb.QueryVerifierAssignmentsRequest{
					Verifier:         clientCtx.GetFromAddress().String(),
					IncludeFinalized: includeFinal,
				})
				if err != nil {
					return err
				}
				nowUnix := time.Now().Unix()
				var matches []*registrypb.PublisherVerificationAssignment
				for _, entry := range resp.GetAssignments() {
					assignment := entry.GetAssignment()
					if assignment == nil {
						continue
					}
					if assignment.GetDomain() != domain {
						continue
					}
					if assignment.GetFinalized() {
						continue
					}
					if nowUnix < assignment.GetStartAtUnix() || nowUnix > assignment.GetDeadlineUnix() {
						continue
					}
					matches = append(matches, assignment)
				}
				if len(matches) == 0 {
					return fmt.Errorf("no active assignment found for %s (specify --round-start to override)", domain)
				}
				if len(matches) > 1 {
					return fmt.Errorf("multiple active assignments found for %s; specify --round-start", domain)
				}
				roundStart = matches[0].GetRoundStartUnix()
				assignmentOwner := strings.TrimSpace(matches[0].GetVerificationOwner())
				if verificationOwner == "" {
					verificationOwner = assignmentOwner
				} else if assignmentOwner != "" && verificationOwner != assignmentOwner {
					return fmt.Errorf("--verification-owner does not match assignment")
				}
			}

			if commitHash == "" {
				if passedFlag == failedFlag {
					return fmt.Errorf("specify exactly one of --passed or --failed when computing commit hash")
				}
				if nonce == "" {
					return fmt.Errorf("--nonce required when computing commit hash")
				}
				passed := passedFlag
				evidenceHash = similar.resolveHash(evidenceHash)
				if strings.TrimSpace(verificationOwner) != "" {
					commitHash = registrypb.ComputeVerificationCommitHashV2(domain, roundStart, clientCtx.GetFromAddress().String(), verificationOwner, passed, evidenceHash, nonce)
				} else {
					commitHash = registrypb.ComputeVerificationCommitHash(domain, roundStart, clientCtx.GetFromAddress().String(), passed, evidenceHash, nonce)
				}
			}

			msg := &registrypb.MsgSubmitVerificationCommit{
				Verifier:       clientCtx.GetFromAddress().String(),
				Domain:         domain,
				RoundStartUnix: roundStart,
				CommitHash:     commitHash,
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().BoolVar(&passedFlag, "passed", false, "mark verification as passed")
	cmd.Flags().BoolVar(&failedFlag, "failed", false, "mark verification as failed")
	cmd.Flags().Int64Var(&roundStart, "round-start", 0, "round start unix timestamp (optional, auto-detected if omitted)")
	cmd.Flags().BoolVar(&includeFinal, "include-finalized", false, "include finalized assignments when auto-resolving round start")
	cmd.Flags().StringVar(&commitHash, "commit-hash", "", "precomputed commit hash (optional if --passed/--failed and --nonce provided)")
	cmd.Flags().StringVar(&evidenceHash, "evidence-hash", "", "optional evidence hash used when computing commit hash")
	cmd.Flags().StringVar(&nonce, "nonce", "", "nonce used when computing commit hash")
	cmd.Flags().StringVar(&verificationOwner, "verification-owner", "", "assignment-scoped publisher wallet bound into the commit hash")
	similar.addTo(cmd)
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func verifierRevealCmd() *cobra.Command {
	var (
		passedFlag   bool
		failedFlag   bool
		roundStart   int64
		includeFinal bool

		evidenceHash      string
		nonce             string
		verificationOwner string
		similar           similarEvidenceFlags
	)
	cmd := &cobra.Command{
		Use:   "reveal [domain]",
		Short: "Reveal a verification result for a committed assignment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			if passedFlag == failedFlag {
				return fmt.Errorf("specify exactly one of --passed or --failed")
			}
			if nonce == "" {
				return fmt.Errorf("--nonce required")
			}
			passed := passedFlag

			domain := registry.NormalizeDomain(args[0])
			if domain == "" {
				return fmt.Errorf("domain required")
			}
			if !registry.IsDomainFormatValid(domain) {
				return fmt.Errorf("domain %s is not a valid hostname", domain)
			}

			if roundStart == 0 {
				qctx, err := client.GetClientQueryContext(cmd)
				if err != nil {
					return err
				}
				queryClient := registrypb.NewQueryClient(qctx)
				resp, err := queryClient.VerifierAssignments(cmd.Context(), &registrypb.QueryVerifierAssignmentsRequest{
					Verifier:         clientCtx.GetFromAddress().String(),
					IncludeFinalized: includeFinal,
				})
				if err != nil {
					return err
				}
				nowUnix := time.Now().Unix()
				var matches []*registrypb.PublisherVerificationAssignment
				for _, entry := range resp.GetAssignments() {
					assignment := entry.GetAssignment()
					if assignment == nil {
						continue
					}
					if assignment.GetDomain() != domain {
						continue
					}
					if assignment.GetFinalized() {
						continue
					}
					if nowUnix < assignment.GetStartAtUnix() || nowUnix > assignment.GetDeadlineUnix() {
						continue
					}
					matches = append(matches, assignment)
				}
				if len(matches) == 0 {
					return fmt.Errorf("no active assignment found for %s (specify --round-start to override)", domain)
				}
				if len(matches) > 1 {
					return fmt.Errorf("multiple active assignments found for %s; specify --round-start", domain)
				}
				roundStart = matches[0].GetRoundStartUnix()
				assignmentOwner := strings.TrimSpace(matches[0].GetVerificationOwner())
				if verificationOwner == "" {
					verificationOwner = assignmentOwner
				} else if assignmentOwner != "" && verificationOwner != assignmentOwner {
					return fmt.Errorf("--verification-owner does not match assignment")
				}
			}

			evidenceHash = similar.resolveHash(evidenceHash)
			msg := &registrypb.MsgRevealVerification{
				Verifier:               clientCtx.GetFromAddress().String(),
				Domain:                 domain,
				RoundStartUnix:         roundStart,
				Passed:                 passed,
				EvidenceHash:           evidenceHash,
				Nonce:                  nonce,
				ObservedSimilarDomains: similar.observed,
				MatchedSimilarDomains:  similar.matched,
				ExpectedSimilarDomains: similar.expected,
				ExpectedSetHash:        similar.expectedSetHash,
				ObservedSetHash:        similar.observedSetHash,
				VerificationOwner:      strings.TrimSpace(verificationOwner),
			}
			if err := msg.ValidateBasic(); err != nil {
				return err
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	cmd.Flags().BoolVar(&passedFlag, "passed", false, "mark verification as passed")
	cmd.Flags().BoolVar(&failedFlag, "failed", false, "mark verification as failed")
	cmd.Flags().Int64Var(&roundStart, "round-start", 0, "round start unix timestamp (optional, auto-detected if omitted)")
	cmd.Flags().BoolVar(&includeFinal, "include-finalized", false, "include finalized assignments when auto-resolving round start")
	cmd.Flags().StringVar(&evidenceHash, "evidence-hash", "", "optional evidence hash (must match commit)")
	cmd.Flags().StringVar(&nonce, "nonce", "", "nonce used for the commit")
	cmd.Flags().StringVar(&verificationOwner, "verification-owner", "", "assignment-scoped publisher wallet checked on the homepage")
	similar.addTo(cmd)
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func verifierAssignmentsCmd() *cobra.Command {
	var includeFinal bool
	var verifier string
	cmd := &cobra.Command{
		Use:   "assignments [verifier]",
		Short: "List verification assignments for a verifier",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				verifier = args[0]
			}
			if verifier == "" {
				txCtx, err := client.GetClientTxContext(cmd)
				if err != nil {
					return err
				}
				verifier = txCtx.GetFromAddress().String()
			}

			qctx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}
			queryClient := registrypb.NewQueryClient(qctx)
			resp, err := queryClient.VerifierAssignments(cmd.Context(), &registrypb.QueryVerifierAssignmentsRequest{
				Verifier:         verifier,
				IncludeFinalized: includeFinal,
			})
			if err != nil {
				return err
			}
			return qctx.PrintProto(resp)
		},
	}
	cmd.Flags().BoolVar(&includeFinal, "include-finalized", false, "include finalized assignments")
	cmd.Flags().StringVar(&verifier, "verifier", "", "verifier bech32 address (defaults to --from)")
	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}
