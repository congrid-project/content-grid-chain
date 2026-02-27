package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	sdkmath "cosmossdk.io/math"
	"github.com/spf13/cobra"

	"content-grid-chain/x/tokenomics"
)

func main() {
	root := &cobra.Command{
		Use:   "tokenomics",
		Short: "Tokenomics planning helpers for Content Grid",
	}

	root.AddCommand(newSimulateCmd())
	root.AddCommand(newGenesisTemplateCmd())
	root.AddCommand(newAirdropCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func newSimulateCmd() *cobra.Command {
	var (
		years        int
		supplyTokens float64
		protocolFees float64
		slashLoss    float64
		outputJSON   bool
	)

	defaultGenesis := tokenomics.DefaultGenesisState()
	supplyTokens = defaultGenesis.InitialSupply.ToLegacyDec().Quo(sdkmath.LegacyNewDec(1_000_000)).MustFloat64()
	protocolFees = 1_500_000
	slashLoss = 100_000
	years = 5

	cmd := &cobra.Command{
		Use:   "simulate",
		Short: "Simulate multi-year linear issuance with adjustable burn assumptions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := tokenomics.SimulationConfig{
				Years:              years,
				InitialSupply:      toMicroDec(supplyTokens),
				AnnualProtocolFees: toMicroDec(protocolFees),
				AnnualSlashingLoss: toMicroDec(slashLoss),
			}

			projections, err := tokenomics.SimulateYears(tokenomics.DefaultParams(), cfg)
			if err != nil {
				return err
			}

			if outputJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(projections)
			}

			tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "YEAR\tSTART(M)\tPUB ISSUED(M)\tVER ISSUED(M)\tISSUED(M)\tISSUED CUM(M)\tFEE BURN(M)\tSLASH BURN(M)\tNET(M)\tEND(M)")
			for _, proj := range projections {
				fmt.Fprintf(
					tw,
					"%d\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\t%.2f\n",
					proj.Year,
					decToMillions(proj.StartSupply),
					decToMillions(proj.PublisherIssued),
					decToMillions(proj.VerifierIssued),
					decToMillions(proj.NewlyIssued),
					decToMillions(proj.CumulativeIssued),
					decToMillions(proj.FeeBurned),
					decToMillions(proj.SlashBurned),
					decToMillions(proj.NetIssuance),
					decToMillions(proj.EndSupply),
				)
			}
			return tw.Flush()
		},
	}

	cmd.Flags().IntVar(&years, "years", years, "number of years to simulate")
	cmd.Flags().Float64Var(&supplyTokens, "supply", supplyTokens, "initial supply in CONGRID tokens")
	cmd.Flags().Float64Var(&protocolFees, "protocol-fees", protocolFees, "annual on-chain protocol fees in CONGRID")
	cmd.Flags().Float64Var(&slashLoss, "slash-loss", slashLoss, "annual slashed stake in CONGRID")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "emit projections as JSON")

	return cmd
}

func newGenesisTemplateCmd() *cobra.Command {
	var (
		outputPath string
		foundation string
		team       string
		verifiers  string
		publishers string
		liquidity  string
		treasury   string
		pretty     bool
	)

	cmd := &cobra.Command{
		Use:   "genesis-template",
		Short: "Emit a genesis allocation template with optional addresses",
		RunE: func(cmd *cobra.Command, _ []string) error {
			gs := tokenomics.DefaultGenesisState()
			update := func(category, addr string) {
				if addr != "" {
					gs = gs.WithAllocationAddress(category, addr)
				}
			}
			update("foundation_reserve", foundation)
			update("team_and_advisors", team)
			update("verifier_bootstrap_pool", verifiers)
			update("publisher_growth_fund", publishers)
			update("public_sale_and_liquidity", liquidity)
			update("community_treasury", treasury)

			if err := gs.Validate(); err != nil {
				return err
			}

			var w io.Writer = os.Stdout
			if outputPath != "" {
				f, err := os.Create(outputPath)
				if err != nil {
					return err
				}
				defer f.Close()
				w = f
			}

			enc := json.NewEncoder(w)
			if pretty {
				enc.SetIndent("", "  ")
			}
			return enc.Encode(gs)
		},
	}

	cmd.Flags().StringVar(&outputPath, "output", "", "write template to file (defaults to stdout)")
	cmd.Flags().StringVar(&foundation, "foundation", "", "foundation reserve address")
	cmd.Flags().StringVar(&team, "team", "", "team and advisors address")
	cmd.Flags().StringVar(&verifiers, "verifiers", "", "verifier bootstrap pool address")
	cmd.Flags().StringVar(&publishers, "publishers", "", "publisher growth fund address")
	cmd.Flags().StringVar(&liquidity, "liquidity", "", "public sale / liquidity address")
	cmd.Flags().StringVar(&treasury, "treasury", "", "community treasury address")
	cmd.Flags().BoolVar(&pretty, "pretty", true, "pretty-print JSON output")

	return cmd
}

func newAirdropCmd() *cobra.Command {
	var (
		inputPath    string
		supplyTokens float64
		denom        string
		hasHeader    bool
		pretty       bool
	)

	supplyTokens = 10_000_000
	denom = tokenomics.DefaultDenom
	hasHeader = true

	cmd := &cobra.Command{
		Use:   "airdrop",
		Short: "Convert a CSV of addresses and weights into allocation amounts",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if inputPath == "" {
				return fmt.Errorf("--input is required")
			}
			file, err := os.Open(inputPath)
			if err != nil {
				return err
			}
			defer file.Close()

			records, err := readCSV(file, hasHeader)
			if err != nil {
				return err
			}
			if len(records) == 0 {
				return fmt.Errorf("input CSV contained no rows")
			}

			totalWeight := sdkmath.LegacyZeroDec()
			for _, rec := range records {
				totalWeight = totalWeight.Add(rec.weight)
			}
			if totalWeight.IsZero() {
				return fmt.Errorf("total weight must be > 0")
			}

			supply := toMicroDec(supplyTokens)
			allocations := make([]airdropAllocation, 0, len(records))
			remaining := supply
			for idx, rec := range records {
				share := rec.weight.Quo(totalWeight)
				amount := share.Mul(supply).TruncateInt()
				if idx == len(records)-1 {
					amount = remaining.TruncateInt()
				} else {
					remaining = remaining.Sub(sdkmath.LegacyNewDecFromInt(amount))
				}
				allocations = append(allocations, airdropAllocation{Address: rec.address, Amount: amount.String()})
			}

			payload := airdropOutput{
				Denom:       denom,
				TotalSupply: supply.RoundInt().String(),
				Allocations: allocations,
			}

			enc := json.NewEncoder(os.Stdout)
			if pretty {
				enc.SetIndent("", "  ")
			}
			return enc.Encode(payload)
		},
	}

	cmd.Flags().StringVar(&inputPath, "input", "", "path to CSV containing address,weight columns")
	cmd.Flags().Float64Var(&supplyTokens, "supply", supplyTokens, "total token supply in CONGRID to distribute")
	cmd.Flags().StringVar(&denom, "denom", denom, "coin denomination for output")
	cmd.Flags().BoolVar(&hasHeader, "header", hasHeader, "input CSV has a header row")
	cmd.Flags().BoolVar(&pretty, "pretty", true, "pretty-print JSON output")

	return cmd
}

type csvRecord struct {
	address string
	weight  sdkmath.LegacyDec
}

type airdropAllocation struct {
	Address string `json:"address"`
	Amount  string `json:"amount"`
}

type airdropOutput struct {
	Denom       string              `json:"denom"`
	TotalSupply string              `json:"total_supply"`
	Allocations []airdropAllocation `json:"allocations"`
}

func readCSV(r io.Reader, hasHeader bool) ([]csvRecord, error) {
	reader := csv.NewReader(r)
	raw, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if hasHeader && len(raw) > 0 {
		raw = raw[1:]
	}
	out := make([]csvRecord, 0, len(raw))
	for i, row := range raw {
		if len(row) < 2 {
			return nil, fmt.Errorf("row %d expected at least 2 columns", i)
		}
		weight, err := sdkmath.LegacyNewDecFromStr(strings.TrimSpace(row[1]))
		if err != nil {
			return nil, fmt.Errorf("row %d invalid weight: %w", i, err)
		}
		out = append(out, csvRecord{address: strings.TrimSpace(row[0]), weight: weight})
	}
	return out, nil
}

func toMicroDec(tokens float64) sdkmath.LegacyDec {
	micro := sdkmath.LegacyNewDec(1_000_000)
	base := sdkmath.LegacyMustNewDecFromStr(fmt.Sprintf("%.6f", tokens))
	return base.Mul(micro)
}

func decToMillions(dec sdkmath.LegacyDec) float64 {
	millionMicro := sdkmath.LegacyNewDec(1_000_000_000_000)
	return dec.Quo(millionMicro).MustFloat64()
}
