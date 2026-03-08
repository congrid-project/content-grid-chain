package tokenomics

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
)

const hoursPerYear = int64(365 * 24)

// SimulationConfig captures the scenario used when simulating supply over time.
type SimulationConfig struct {
	Years              int
	InitialSupply      sdkmath.LegacyDec
	AnnualProtocolFees sdkmath.LegacyDec
	AnnualSlashingLoss sdkmath.LegacyDec
}

// DefaultSimulationConfig returns a conservative baseline scenario spanning 5 years.
func DefaultSimulationConfig(gs GenesisState) SimulationConfig {
	supply := sdkmath.LegacyNewDecFromInt(gs.InitialSupply)
	micro := sdkmath.LegacyNewDec(1_000_000)
	toMicro := func(amount int64) sdkmath.LegacyDec {
		return sdkmath.LegacyNewDec(amount).Mul(micro)
	}
	return SimulationConfig{
		Years:              5,
		InitialSupply:      supply,
		AnnualProtocolFees: toMicro(1_500_000),
		AnnualSlashingLoss: toMicro(100_000),
	}
}

// YearProjection captures the monetary movements for a given simulation year.
type YearProjection struct {
	Year             int               `json:"year"`
	StartSupply      sdkmath.LegacyDec `json:"start_supply"`
	OperatorIssued   sdkmath.LegacyDec `json:"operator_issued"`
	PublisherIssued  sdkmath.LegacyDec `json:"publisher_issued"`
	VerifierIssued   sdkmath.LegacyDec `json:"verifier_issued"`
	NewlyIssued      sdkmath.LegacyDec `json:"newly_issued"`
	CumulativeIssued sdkmath.LegacyDec `json:"cumulative_issued"`
	FeeBurned        sdkmath.LegacyDec `json:"fee_burned"`
	SlashBurned      sdkmath.LegacyDec `json:"slash_burned"`
	NetIssuance      sdkmath.LegacyDec `json:"net_issuance"`
	EndSupply        sdkmath.LegacyDec `json:"end_supply"`
}

// SimulateYears returns yearly projections under the provided params and scenario.
func SimulateYears(params Params, cfg SimulationConfig) ([]YearProjection, error) {
	if cfg.Years <= 0 {
		return nil, fmt.Errorf("years must be positive")
	}
	if cfg.InitialSupply.IsNegative() {
		return nil, fmt.Errorf("initial supply must be non-negative")
	}
	if cfg.AnnualProtocolFees.IsNegative() || cfg.AnnualSlashingLoss.IsNegative() {
		return nil, fmt.Errorf("annual inputs must be non-negative")
	}

	if err := params.Validate(); err != nil {
		return nil, err
	}

	projections := make([]YearProjection, 0, cfg.Years)
	supply := cfg.InitialSupply
	cumulativeIssued := sdkmath.LegacyZeroDec()
	totalDurationHours := params.Issuance.EmissionDurationHours
	remainingHours := totalDurationHours
	totalSupply := sdkmath.LegacyNewDecFromInt(params.Issuance.EmissionTotalSupply)
	durationDec := sdkmath.LegacyNewDec(totalDurationHours * 10_000)

	feeBurnPercent := params.FeeSplit.ToBurn
	slashBurnPercent := params.SlashSplit.ToBurn

	for year := 1; year <= cfg.Years; year++ {
		yearHours := hoursPerYear
		if remainingHours < yearHours {
			yearHours = remainingHours
		}
		operatorIssued := sdkmath.LegacyZeroDec()
		publisherIssued := sdkmath.LegacyZeroDec()
		verifierIssued := sdkmath.LegacyZeroDec()
		if yearHours > 0 {
			yearHoursDec := sdkmath.LegacyNewDec(yearHours)
			operatorIssued = totalSupply.MulInt64(params.Issuance.OperatorReserveBps).Mul(yearHoursDec).Quo(durationDec)
			publisherIssued = totalSupply.MulInt64(params.Issuance.PublisherEmissionBps).Mul(yearHoursDec).Quo(durationDec)
			verifierIssued = totalSupply.MulInt64(params.Issuance.VerifierEmissionBps).Mul(yearHoursDec).Quo(durationDec)
			remainingHours -= yearHours
		}

		newlyIssued := operatorIssued.Add(publisherIssued).Add(verifierIssued)
		cumulativeIssued = cumulativeIssued.Add(newlyIssued)
		feeBurn := cfg.AnnualProtocolFees.Mul(feeBurnPercent)
		slashBurn := cfg.AnnualSlashingLoss.Mul(slashBurnPercent)
		net := newlyIssued.Sub(feeBurn).Sub(slashBurn)
		endSupply := supply.Add(net)

		projections = append(projections, YearProjection{
			Year:             year,
			StartSupply:      supply,
			OperatorIssued:   operatorIssued,
			PublisherIssued:  publisherIssued,
			VerifierIssued:   verifierIssued,
			NewlyIssued:      newlyIssued,
			CumulativeIssued: cumulativeIssued,
			FeeBurned:        feeBurn,
			SlashBurned:      slashBurn,
			NetIssuance:      net,
			EndSupply:        endSupply,
		})
		supply = endSupply
	}

	return projections, nil
}

// ProjectionTable renders the simulation into a human readable table-ready structure.
func ProjectionTable(projections []YearProjection) [][]string {
	rows := make([][]string, 0, len(projections)+1)
	header := []string{"Year", "Start Supply", "Operator Issued", "Publisher Issued", "Verifier Issued", "Issued", "Issued (cum)", "Burn (fees)", "Burn (slash)", "Net", "End Supply"}
	rows = append(rows, header)
	for _, proj := range projections {
		rows = append(rows, []string{
			fmt.Sprintf("%d", proj.Year),
			formatDec(proj.StartSupply),
			formatDec(proj.OperatorIssued),
			formatDec(proj.PublisherIssued),
			formatDec(proj.VerifierIssued),
			formatDec(proj.NewlyIssued),
			formatDec(proj.CumulativeIssued),
			formatDec(proj.FeeBurned),
			formatDec(proj.SlashBurned),
			formatDec(proj.NetIssuance),
			formatDec(proj.EndSupply),
		})
	}
	return rows
}

func formatDec(dec sdkmath.LegacyDec) string {
	micro := sdkmath.LegacyNewDec(1_000_000)
	million := sdkmath.LegacyNewDec(1_000_000)
	denom := micro.Mul(million)
	rounded := dec.Quo(denom)
	return fmt.Sprintf("%.2f M", rounded.MustFloat64())
}

// TokenAmount computes the integer token amount for a given percentage of supply.
func TokenAmount(supply sdkmath.Int, percent sdkmath.LegacyDec) sdkmath.Int {
	return percent.MulInt(supply).TruncateInt()
}
