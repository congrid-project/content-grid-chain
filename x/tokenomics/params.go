package tokenomics

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
)

// Params capture all configurable knobs for the Content Grid monetary policy.
type Params struct {
	Issuance   IssuanceParams `json:"issuance"`
	FeeSplit   FeeSplit       `json:"fee_split"`
	SlashSplit SlashSplit     `json:"slash_split"`
}

// DefaultParams returns the monetary policy described in the tokenomics plan.
func DefaultParams() Params {
	return Params{
		Issuance: IssuanceParams{
			EmissionTotalSupply:   sdkmath.NewInt(1_000_000_000_000000), // 1B CONGRID in ucongrid
			OperatorReserveBps:    4000,
			PublisherEmissionBps:  1000,
			VerifierEmissionBps:   5000,
			EmissionDurationHours: 100 * 365 * 24,
		},
		FeeSplit: FeeSplit{
			ToCommunityPool: mustNewDec("0.80"),
			ToBurn:          mustNewDec("0.20"),
		},
		SlashSplit: SlashSplit{
			ToBurn:          mustNewDec("0.50"),
			ToVictimComp:    mustNewDec("0.30"),
			ToCommunityPool: mustNewDec("0.20"),
		},
	}
}

// Validate ensures all params are in range and self-consistent.
func (p Params) Validate() error {
	if err := p.Issuance.Validate(); err != nil {
		return fmt.Errorf("invalid issuance params: %w", err)
	}
	if err := p.FeeSplit.Validate(); err != nil {
		return err
	}
	if err := p.SlashSplit.Validate(); err != nil {
		return err
	}
	return nil
}

// IssuanceParams define fixed-supply linear release for operator/publisher/verifier buckets.
type IssuanceParams struct {
	EmissionTotalSupply   sdkmath.Int `json:"emission_total_supply"`
	OperatorReserveBps    int64       `json:"operator_reserve_bps"`
	PublisherEmissionBps  int64       `json:"publisher_emission_bps"`
	VerifierEmissionBps   int64       `json:"verifier_emission_bps"`
	EmissionDurationHours int64       `json:"emission_duration_hours"`
}

// Validate sanity checks the fixed linear issuance configuration.
func (ip IssuanceParams) Validate() error {
	if !ip.EmissionTotalSupply.IsPositive() {
		return fmt.Errorf("emission total supply must be positive")
	}
	if ip.EmissionDurationHours <= 0 {
		return fmt.Errorf("emission duration hours must be positive")
	}
	if err := ensureBps(ip.OperatorReserveBps, "operator reserve bps"); err != nil {
		return err
	}
	if err := ensureBps(ip.PublisherEmissionBps, "publisher emission bps"); err != nil {
		return err
	}
	if err := ensureBps(ip.VerifierEmissionBps, "verifier emission bps"); err != nil {
		return err
	}
	if ip.OperatorReserveBps+ip.PublisherEmissionBps+ip.VerifierEmissionBps != 10_000 {
		return fmt.Errorf("issuance bps must sum to 10000")
	}
	return nil
}

// FeeSplit defines how protocol fees get redistributed.
type FeeSplit struct {
	ToCommunityPool sdkmath.LegacyDec `json:"to_community_pool"`
	ToBurn          sdkmath.LegacyDec `json:"to_burn"`
}

// Validate ensures the fee split sums to 1.
func (fs FeeSplit) Validate() error {
	shares := []sdkmath.LegacyDec{fs.ToCommunityPool, fs.ToBurn}
	if err := ensureSharesSumToOne(shares, "fee split"); err != nil {
		return err
	}
	return nil
}

// SlashSplit defines distribution of slashed stake.
type SlashSplit struct {
	ToBurn          sdkmath.LegacyDec `json:"to_burn"`
	ToVictimComp    sdkmath.LegacyDec `json:"to_victim_comp"`
	ToCommunityPool sdkmath.LegacyDec `json:"to_community_pool"`
}

// Validate ensures the slash split sums to 1.
func (ss SlashSplit) Validate() error {
	shares := []sdkmath.LegacyDec{ss.ToBurn, ss.ToVictimComp, ss.ToCommunityPool}
	if err := ensureSharesSumToOne(shares, "slash split"); err != nil {
		return err
	}
	return nil
}

func ensureSharesSumToOne(shares []sdkmath.LegacyDec, label string) error {
	total := sdkmath.LegacyZeroDec()
	for _, dec := range shares {
		if err := ensureUnitInterval(dec, label+" component"); err != nil {
			return err
		}
		total = total.Add(dec)
	}
	if !total.Equal(sdkmath.LegacyOneDec()) {
		tolerance := mustNewDec("0.000000001")
		if total.Sub(sdkmath.LegacyOneDec()).Abs().GT(tolerance) {
			return fmt.Errorf("%s shares must sum to 1. got %s", label, total)
		}
	}
	return nil
}

func ensureUnitInterval(dec sdkmath.LegacyDec, label string) error {
	if dec.IsNegative() {
		return fmt.Errorf("%s must be >= 0", label)
	}
	if dec.GT(sdkmath.LegacyOneDec()) {
		return fmt.Errorf("%s must be <= 1", label)
	}
	return nil
}

func ensureBps(value int64, label string) error {
	if value < 0 || value > 10_000 {
		return fmt.Errorf("%s must be within [0,10000]", label)
	}
	return nil
}

func mustNewDec(val string) sdkmath.LegacyDec {
	return sdkmath.LegacyMustNewDecFromStr(val)
}
