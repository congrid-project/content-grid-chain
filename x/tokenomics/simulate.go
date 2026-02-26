package tokenomics

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
)

// InflationRate returns the annualized inflation for the provided bonded ratio.
func (ip InflationParams) InflationRate(bondedRatio sdkmath.LegacyDec) sdkmath.LegacyDec {
	if bondedRatio.IsNegative() {
		bondedRatio = sdkmath.LegacyZeroDec()
	}
	if bondedRatio.LTE(ip.TargetBondedLow) {
		return ip.MaxRate
	}
	if bondedRatio.GTE(ip.TargetBondedHigh) {
		return ip.MinRate
	}

	midpoint := ip.TargetBondedLow.Add(ip.TargetBondedHigh).QuoInt64(2)
	if bondedRatio.LTE(midpoint) {
		span := midpoint.Sub(ip.TargetBondedLow)
		if span.IsZero() {
			return ip.BaseRate
		}
		progress := bondedRatio.Sub(ip.TargetBondedLow).Quo(span)
		return ip.MaxRate.Sub(ip.MaxRate.Sub(ip.BaseRate).Mul(progress))
	}

	span := ip.TargetBondedHigh.Sub(midpoint)
	if span.IsZero() {
		return ip.BaseRate
	}
	progress := bondedRatio.Sub(midpoint).Quo(span)
	return ip.BaseRate.Sub(ip.BaseRate.Sub(ip.MinRate).Mul(progress))
}

// SimulationConfig captures the scenario used when simulating supply over time.
type SimulationConfig struct {
	Years              int
	InitialSupply      sdkmath.LegacyDec
	BondedRatio        sdkmath.LegacyDec
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
		BondedRatio:        mustNewDec("0.60"),
		AnnualProtocolFees: toMicro(15_000_000),
		AnnualSlashingLoss: toMicro(1_000_000),
	}
}

// YearProjection captures the monetary movements for a given simulation year.
type YearProjection struct {
	Year          int               `json:"year"`
	StartSupply   sdkmath.LegacyDec `json:"start_supply"`
	InflationRate sdkmath.LegacyDec `json:"inflation_rate"`
	NewlyMinted   sdkmath.LegacyDec `json:"newly_minted"`
	FeeBurned     sdkmath.LegacyDec `json:"fee_burned"`
	SlashBurned   sdkmath.LegacyDec `json:"slash_burned"`
	NetIssuance   sdkmath.LegacyDec `json:"net_issuance"`
	EndSupply     sdkmath.LegacyDec `json:"end_supply"`
}

// SimulateYears returns yearly projections under the provided params and scenario.
func SimulateYears(params Params, cfg SimulationConfig) ([]YearProjection, error) {
	if cfg.Years <= 0 {
		return nil, fmt.Errorf("years must be positive")
	}
	if cfg.InitialSupply.IsNegative() {
		return nil, fmt.Errorf("initial supply must be non-negative")
	}
	if cfg.BondedRatio.IsNegative() || cfg.BondedRatio.GT(sdkmath.LegacyOneDec()) {
		return nil, fmt.Errorf("bonded ratio must be within [0,1]")
	}
	if cfg.AnnualProtocolFees.IsNegative() || cfg.AnnualSlashingLoss.IsNegative() {
		return nil, fmt.Errorf("annual inputs must be non-negative")
	}

	if err := params.Validate(); err != nil {
		return nil, err
	}

	projections := make([]YearProjection, 0, cfg.Years)
	supply := cfg.InitialSupply
	feeBurnPercent := params.FeeSplit.ToBurn
	slashBurnPercent := params.SlashSplit.ToBurn

	for year := 1; year <= cfg.Years; year++ {
		rate := params.Inflation.InflationRate(cfg.BondedRatio)
		minted := supply.Mul(rate)
		feeBurn := cfg.AnnualProtocolFees.Mul(feeBurnPercent)
		slashBurn := cfg.AnnualSlashingLoss.Mul(slashBurnPercent)
		net := minted.Sub(feeBurn).Sub(slashBurn)
		endSupply := supply.Add(net)

		projections = append(projections, YearProjection{
			Year:          year,
			StartSupply:   supply,
			InflationRate: rate,
			NewlyMinted:   minted,
			FeeBurned:     feeBurn,
			SlashBurned:   slashBurn,
			NetIssuance:   net,
			EndSupply:     endSupply,
		})
		supply = endSupply
	}

	return projections, nil
}

// ProjectionTable renders the simulation into a human readable table-ready structure.
func ProjectionTable(projections []YearProjection) [][]string {
	rows := make([][]string, 0, len(projections)+1)
	header := []string{"Year", "Start Supply", "Inflation", "Minted", "Burn (fees)", "Burn (slash)", "Net", "End Supply"}
	rows = append(rows, header)
	for _, proj := range projections {
		rows = append(rows, []string{
			fmt.Sprintf("%d", proj.Year),
			formatDec(proj.StartSupply),
			fmt.Sprintf("%.2f%%", proj.InflationRate.MulInt64(100).MustFloat64()),
			formatDec(proj.NewlyMinted),
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
