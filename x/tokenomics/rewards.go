package tokenomics

import (
	sdkmath "cosmossdk.io/math"
)

// BlockRewardAllocation captures how minted tokens are split in a block.
type BlockRewardAllocation struct {
	Staking    sdkmath.LegacyDec `json:"staking"`
	Publishers sdkmath.LegacyDec `json:"publishers"`
	Community  sdkmath.LegacyDec `json:"community"`
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

// BlockRewards returns the block-level allocation totals for the provided amount.
func (p Params) SplitBlockRewards(amount sdkmath.LegacyDec) BlockRewardAllocation {
	return BlockRewardAllocation{
		Staking:    amount.Mul(p.BlockRewards.StakingRewards),
		Publishers: amount.Mul(p.BlockRewards.PublisherRewards),
		Community:  amount.Mul(p.BlockRewards.CommunityPool),
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
