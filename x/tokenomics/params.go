package tokenomics

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
)

// Params capture all configurable knobs for the Content Grid monetary policy.
type Params struct {
	Inflation    InflationParams  `json:"inflation"`
	BlockRewards BlockRewardSplit `json:"block_rewards"`
	FeeSplit     FeeSplit         `json:"fee_split"`
	SlashSplit   SlashSplit       `json:"slash_split"`
}

// DefaultParams returns the monetary policy described in the tokenomics plan.
func DefaultParams() Params {
	return Params{
		Inflation: InflationParams{
			BaseRate:         mustNewDec("0.07"),
			MinRate:          mustNewDec("0.04"),
			MaxRate:          mustNewDec("0.12"),
			TargetBondedLow:  mustNewDec("0.50"),
			TargetBondedHigh: mustNewDec("0.70"),
			BlocksPerYear:    5_256_000, // 365 days * 24h * 60m * 60s / 6s block time
		},
		BlockRewards: BlockRewardSplit{
			StakingRewards:   mustNewDec("0.25"),
			PublisherRewards: mustNewDec("0.65"),
			CommunityPool:    mustNewDec("0.10"),
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
	if err := p.Inflation.Validate(); err != nil {
		return fmt.Errorf("invalid inflation params: %w", err)
	}
	if err := p.BlockRewards.Validate("block rewards"); err != nil {
		return err
	}
	if err := p.FeeSplit.Validate(); err != nil {
		return err
	}
	if err := p.SlashSplit.Validate(); err != nil {
		return err
	}
	return nil
}

// InflationParams determine how annual inflation evolves with the bonded ratio.
type InflationParams struct {
	BaseRate         sdkmath.LegacyDec `json:"base_rate"`
	MinRate          sdkmath.LegacyDec `json:"min_rate"`
	MaxRate          sdkmath.LegacyDec `json:"max_rate"`
	TargetBondedLow  sdkmath.LegacyDec `json:"target_bonded_low"`
	TargetBondedHigh sdkmath.LegacyDec `json:"target_bonded_high"`
	BlocksPerYear    uint64            `json:"blocks_per_year"`
}

// Validate sanity checks the inflation configuration.
func (ip InflationParams) Validate() error {
	if err := ensureUnitInterval(ip.BaseRate, "base rate"); err != nil {
		return err
	}
	if err := ensureUnitInterval(ip.MinRate, "min rate"); err != nil {
		return err
	}
	if err := ensureUnitInterval(ip.MaxRate, "max rate"); err != nil {
		return err
	}
	if err := ensureUnitInterval(ip.TargetBondedLow, "target bonded low"); err != nil {
		return err
	}
	if err := ensureUnitInterval(ip.TargetBondedHigh, "target bonded high"); err != nil {
		return err
	}
	if !ip.MaxRate.GTE(ip.BaseRate) {
		return fmt.Errorf("max rate %.4f must be >= base rate %.4f", ip.MaxRate, ip.BaseRate)
	}
	if !ip.BaseRate.GTE(ip.MinRate) {
		return fmt.Errorf("base rate %.4f must be >= min rate %.4f", ip.BaseRate, ip.MinRate)
	}
	if !ip.TargetBondedHigh.GTE(ip.TargetBondedLow) {
		return fmt.Errorf("target bonded high %.4f must be >= low %.4f", ip.TargetBondedHigh, ip.TargetBondedLow)
	}
	if ip.BlocksPerYear == 0 {
		return fmt.Errorf("blocks per year must be positive")
	}
	return nil
}

// BlockRewardSplit represents how freshly minted tokens are routed each block.
type BlockRewardSplit struct {
	StakingRewards   sdkmath.LegacyDec `json:"staking_rewards"`
	PublisherRewards sdkmath.LegacyDec `json:"publisher_rewards"`
	CommunityPool    sdkmath.LegacyDec `json:"community_pool"`
}

// Validate ensures the split sums to 1.
func (br BlockRewardSplit) Validate(name string) error {
	shares := []sdkmath.LegacyDec{br.StakingRewards, br.PublisherRewards, br.CommunityPool}
	if err := ensureSharesSumToOne(shares, name); err != nil {
		return err
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

func mustNewDec(val string) sdkmath.LegacyDec {
	return sdkmath.LegacyMustNewDecFromStr(val)
}
