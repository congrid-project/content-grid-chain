package tokenomics

import (
	sdkmath "cosmossdk.io/math"
)

// BlockRewardAllocation captures how minted tokens are split in a block.
type BlockRewardAllocation struct {
	Tasks      sdkmath.LegacyDec `json:"tasks"`
	Staking    sdkmath.LegacyDec `json:"staking"`
	Publishers sdkmath.LegacyDec `json:"publishers"`
	Community  sdkmath.LegacyDec `json:"community"`
}

// TaskRewardAllocation splits the task bucket between executors and validators.
type TaskRewardAllocation struct {
	Execution  sdkmath.LegacyDec `json:"execution"`
	Validation sdkmath.LegacyDec `json:"validation"`
}

// FeeAllocation describes where protocol fees flow.
type FeeAllocation struct {
	ToTaskRewards sdkmath.LegacyDec `json:"to_task_rewards"`
	Burned        sdkmath.LegacyDec `json:"burned"`
}

// ConsumerPaymentAllocation describes settlement of consumer spend (e.g. bounty payouts).
type ConsumerPaymentAllocation struct {
	ToExecution sdkmath.LegacyDec `json:"to_execution"`
	ToCommunity sdkmath.LegacyDec `json:"to_community"`
	Burned      sdkmath.LegacyDec `json:"burned"`
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
		Tasks:      amount.Mul(p.BlockRewards.TaskRewards),
		Staking:    amount.Mul(p.BlockRewards.StakingRewards),
		Publishers: amount.Mul(p.BlockRewards.PublisherRewards),
		Community:  amount.Mul(p.BlockRewards.CommunityPool),
	}
}

// TaskRewards breaks down the task bucket further.
func (p Params) SplitTaskRewards(amount sdkmath.LegacyDec) TaskRewardAllocation {
	return TaskRewardAllocation{
		Execution:  amount.Mul(p.TaskRewardSplit.Execution),
		Validation: amount.Mul(p.TaskRewardSplit.Validation),
	}
}

// FeeRouting computes how protocol fees are reallocated.
func (p Params) RouteFees(fees sdkmath.LegacyDec) FeeAllocation {
	return FeeAllocation{
		ToTaskRewards: fees.Mul(p.FeeSplit.ToTaskRewards),
		Burned:        fees.Mul(p.FeeSplit.ToBurn),
	}
}

// ConsumerRouting computes the settlement of paid consumer tasks.
func (p Params) RouteConsumerPayments(amount sdkmath.LegacyDec) ConsumerPaymentAllocation {
	return ConsumerPaymentAllocation{
		ToExecution: amount.Mul(p.ConsumerSplit.ToExecution),
		ToCommunity: amount.Mul(p.ConsumerSplit.ToCommunityPool),
		Burned:      amount.Mul(p.ConsumerSplit.ToBurn),
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
