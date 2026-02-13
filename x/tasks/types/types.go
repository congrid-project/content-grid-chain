package types

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
)

// Params capture configuration used during task assignment and consensus.
type Params struct {
	MaxAssignments int `json:"max_assignments"`
	QuorumPercent  int `json:"quorum_percent"`
}

// DefaultParams returns sane defaults for a small validator set.
func DefaultParams() Params {
	return Params{
		MaxAssignments: 3,
		QuorumPercent:  67,
	}
}

// Validate ensures the params lie within safe ranges.
func (p Params) Validate() error {
	if p.MaxAssignments <= 0 {
		return fmt.Errorf("max assignments must be positive")
	}
	if p.QuorumPercent <= 0 || p.QuorumPercent > 100 {
		return fmt.Errorf("quorum percent must be in (0,100], got %d", p.QuorumPercent)
	}
	return nil
}

// GenesisState defines the module genesis configuration.
type GenesisState struct {
	Params Params `json:"params"`
}

// DefaultGenesis returns valid default genesis state.
func DefaultGenesis() *GenesisState {
	return &GenesisState{Params: DefaultParams()}
}

// Validate performs basic sanity checks.
func (gs GenesisState) Validate() error {
	return gs.Params.Validate()
}

// TaskStatus enumerates the lifecycle of a task.
type TaskStatus int32

const (
	TaskStatusUnspecified TaskStatus = 0
	TaskStatusPending     TaskStatus = 1
	TaskStatusCompleted   TaskStatus = 2
	TaskStatusFailed      TaskStatus = 3
)

// Task represents a unit of work (e.g. query or verification).
type Task struct {
	ID          string     `json:"id"`
	Type        string     `json:"type"` // e.g. "query", "verify"
	Payload     string     `json:"payload"`
	Status      TaskStatus `json:"status"`
	ResultHash  string     `json:"result_hash,omitempty"`
	CreatedAt   int64      `json:"created_at"`
	CompletedAt int64      `json:"completed_at,omitempty"`
}

// Assignment captures the deterministic worker selection result for a task.
type Assignment struct {
	TaskID      string   `json:"task_id"`
	BlockHeight int64    `json:"block_height"`
	BlockHash   string   `json:"block_hash"`
	Workers     []string `json:"workers"`
}

// TaskSubmission captures a worker's submitted result hash.
type TaskSubmission struct {
	TaskID     string `json:"task_id"`
	Worker     string `json:"worker"`
	ResultHash string `json:"result_hash"`
}

// ConsensusResult aggregates submissions.
type ConsensusResult struct {
	TaskID      string   `json:"task_id"`
	ResultHash  string   `json:"result_hash"`
	Workers     []string `json:"workers"` // Workers who submitted the winning hash
	TotalCount  int      `json:"total_count"`
	QuorumCount int      `json:"quorum_count"`
	Achieved    bool     `json:"achieved"`
}

// RewardWeights define how worker performance metrics map into incentive weights.
type RewardWeights struct {
	Success      sdkmath.LegacyDec `json:"success"`
	Consensus    sdkmath.LegacyDec `json:"consensus"`
	Latency      sdkmath.LegacyDec `json:"latency"`
	Availability sdkmath.LegacyDec `json:"availability"`
}

// DefaultRewardWeights gives a bias towards successful, low-latency work.
func DefaultRewardWeights() RewardWeights {
	return RewardWeights{
		Success:      mustNewDec("0.40"),
		Consensus:    mustNewDec("0.25"),
		Latency:      mustNewDec("0.20"),
		Availability: mustNewDec("0.15"),
	}
}

// Validate ensures the weights are within [0,1] and sum to 1.
func (rw RewardWeights) Validate() error {
	weights := []sdkmath.LegacyDec{rw.Success, rw.Consensus, rw.Latency, rw.Availability}
	return ensureSharesSumToOne(weights, "reward weights")
}

// WorkerPerformance captures KPI snapshots used for reward computation.
type WorkerPerformance struct {
	Assignments     int   `json:"assignments"`
	Successful      int   `json:"successful"`
	Consensus       int   `json:"consensus"`
	MedianLatencyMs int64 `json:"median_latency_ms"`
	TargetLatencyMs int64 `json:"target_latency_ms"`
	OnlineBlocks    int   `json:"online_blocks"`
	ExpectedOnline  int   `json:"expected_online"`
}

// RewardScore converts the snapshot into a 0-1 normalized weight.
func (wp WorkerPerformance) RewardScore(weights RewardWeights) (sdkmath.LegacyDec, error) {
	if err := weights.Validate(); err != nil {
		return sdkmath.LegacyDec{}, err
	}

	assignments := sdkmath.LegacyNewDec(int64(maxInt(wp.Assignments, 0)))
	success := sdkmath.LegacyZeroDec()
	consensus := sdkmath.LegacyZeroDec()
	if !assignments.IsZero() {
		success = sdkmath.LegacyNewDec(int64(maxInt(wp.Successful, 0))).Quo(assignments)
		consensus = sdkmath.LegacyNewDec(int64(maxInt(wp.Consensus, 0))).Quo(assignments)
		if success.GT(sdkmath.LegacyOneDec()) {
			success = sdkmath.LegacyOneDec()
		}
		if consensus.GT(sdkmath.LegacyOneDec()) {
			consensus = sdkmath.LegacyOneDec()
		}
	}

	latencyScore := sdkmath.LegacyZeroDec()
	if !assignments.IsZero() {
		targetLatency := maxInt64(wp.TargetLatencyMs, 6000)
		observedLatency := maxInt64(wp.MedianLatencyMs, 1)
		if observedLatency < targetLatency {
			observedLatency = targetLatency
		}
		latencyScore = sdkmath.LegacyNewDec(targetLatency).QuoInt64(observedLatency)
		if latencyScore.GT(sdkmath.LegacyOneDec()) {
			latencyScore = sdkmath.LegacyOneDec()
		}
	}

	online := sdkmath.LegacyNewDec(int64(maxInt(wp.OnlineBlocks, 0)))
	expected := sdkmath.LegacyNewDec(int64(maxInt(wp.ExpectedOnline, 0)))
	availability := sdkmath.LegacyZeroDec()
	if !expected.IsZero() {
		availability = online.Quo(expected)
		if availability.GT(sdkmath.LegacyOneDec()) {
			availability = sdkmath.LegacyOneDec()
		}
	}

	score := sdkmath.LegacyZeroDec()
	score = score.Add(weights.Success.Mul(success))
	score = score.Add(weights.Consensus.Mul(consensus))
	score = score.Add(weights.Latency.Mul(latencyScore))
	score = score.Add(weights.Availability.Mul(availability))

	return score, nil
}

func mustNewDec(val string) sdkmath.LegacyDec {
	return sdkmath.LegacyMustNewDecFromStr(val)
}

func ensureSharesSumToOne(shares []sdkmath.LegacyDec, label string) error {
	total := sdkmath.LegacyZeroDec()
	for _, dec := range shares {
		if dec.IsNegative() {
			return fmt.Errorf("%s must be non-negative", label)
		}
		if dec.GT(sdkmath.LegacyOneDec()) {
			return fmt.Errorf("%s component must be <= 1", label)
		}
		total = total.Add(dec)
	}
	if !total.Equal(sdkmath.LegacyOneDec()) {
		return fmt.Errorf("%s weights must sum to 1, got %s", label, total)
	}
	return nil
}

func maxInt(value, floor int) int {
	if value < floor {
		return floor
	}
	return value
}

func maxInt64(value, floor int64) int64 {
	if value < floor {
		return floor
	}
	return value
}
