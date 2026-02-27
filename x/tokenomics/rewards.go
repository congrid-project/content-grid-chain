package tokenomics

import (
	sdkmath "cosmossdk.io/math"
)

// IssuanceAllocation captures how an issuance amount is split by fixed bps buckets.
type IssuanceAllocation struct {
	OperatorReserve sdkmath.LegacyDec `json:"operator_reserve"`
	Publishers      sdkmath.LegacyDec `json:"publishers"`
	Verifiers       sdkmath.LegacyDec `json:"verifiers"`
}

// FeeAllocation describes where protocol fees flow.
type FeeAllocation struct {
	ToCommunityPool sdkmath.LegacyDec `json:"to_community_pool"`
	Burned          sdkmath.LegacyDec `json:"burned"`
}

// SlashAllocation represents how confiscated stake gets redirected.
type SlashAllocation struct {
	Burned      sdkmath.LegacyDec `json:"burned"`
	ToVictims   sdkmath.LegacyDec `json:"to_victims"`
	ToCommunity sdkmath.LegacyDec `json:"to_community"`
}

// SplitIssuance returns allocation totals for the provided amount.
func (p Params) SplitIssuance(amount sdkmath.LegacyDec) IssuanceAllocation {
	return IssuanceAllocation{
		OperatorReserve: amount.MulInt64(p.Issuance.OperatorReserveBps).QuoInt64(10_000),
		Publishers:      amount.MulInt64(p.Issuance.PublisherEmissionBps).QuoInt64(10_000),
		Verifiers:       amount.MulInt64(p.Issuance.VerifierEmissionBps).QuoInt64(10_000),
	}
}

// FeeRouting computes how protocol fees are reallocated.
func (p Params) RouteFees(fees sdkmath.LegacyDec) FeeAllocation {
	return FeeAllocation{
		ToCommunityPool: fees.Mul(p.FeeSplit.ToCommunityPool),
		Burned:          fees.Mul(p.FeeSplit.ToBurn),
	}
}

// SlashRouting returns the split for slashed stake.
func (p Params) RouteSlash(amount sdkmath.LegacyDec) SlashAllocation {
	return SlashAllocation{
		Burned:      amount.Mul(p.SlashSplit.ToBurn),
		ToVictims:   amount.Mul(p.SlashSplit.ToVictimComp),
		ToCommunity: amount.Mul(p.SlashSplit.ToCommunityPool),
	}
}
