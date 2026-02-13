package keeper

import (
	"fmt"

	"content-grid-chain/x/tasks/types"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ProcessSubmission handles a worker's result submission and checks for consensus.
func (k Keeper) ProcessSubmission(ctx sdk.Context, submission types.TaskSubmission) (types.ConsensusResult, error) {
	// 1. Retrieve existing submissions for this task
	// In a real implementation, we would store these in the KVStore.
	// For this prototype, we'll assume we have a helper or just simulate the logic.
	// Let's assume we store submissions in a list.

	submissions := k.GetSubmissions(ctx, submission.TaskID)

	// Check if worker already submitted
	for _, s := range submissions {
		if s.Worker == submission.Worker {
			return types.ConsensusResult{}, fmt.Errorf("worker already submitted")
		}
	}

	// Add new submission
	submissions = append(submissions, submission)
	k.SetSubmissions(ctx, submission.TaskID, submissions)

	// 2. Check Quorum
	params := k.Params(ctx)
	// We need to know the total assigned workers to calculate quorum.
	// Assuming we stored the assignment or can re-calculate/fetch it.
	assignment, found := k.GetAssignment(ctx, submission.TaskID)
	if !found {
		return types.ConsensusResult{}, fmt.Errorf("assignment not found for task %s", submission.TaskID)
	}

	totalAssigned := len(assignment.Workers)
	if totalAssigned == 0 {
		return types.ConsensusResult{}, fmt.Errorf("no workers assigned")
	}

	// Calculate required quorum count
	// e.g. 67% of 5 = 3.35 -> 4
	quorumCount := (totalAssigned * params.QuorumPercent) / 100
	if (totalAssigned*params.QuorumPercent)%100 != 0 {
		quorumCount++
	}

	if len(submissions) < quorumCount {
		// Not enough submissions yet
		return types.ConsensusResult{
			TaskID:      submission.TaskID,
			Achieved:    false,
			TotalCount:  len(submissions),
			QuorumCount: quorumCount,
		}, nil
	}

	// 3. Determine Majority
	// Count votes for each hash
	voteCounts := make(map[string]int)
	for _, s := range submissions {
		voteCounts[s.ResultHash]++
	}

	var majorityHash string
	var maxVotes int
	for hash, count := range voteCounts {
		if count > maxVotes {
			maxVotes = count
			majorityHash = hash
		}
	}

	// Check if majority meets quorum
	if maxVotes >= quorumCount {
		// Consensus Achieved!

		// Identify winners
		var winners []string
		for _, s := range submissions {
			if s.ResultHash == majorityHash {
				winners = append(winners, s.Worker)
			}
		}

		// Distribute rewards to winners
		// In a real implementation, the reward amount would come from tokenomics params or block budget.
		// For this prototype, we'll use a placeholder value of 1000 ucongrid.
		rewardAmount := sdkmath.NewInt(1000)
		_ = k.tokenomicsKeep.DistributeTaskRewards(ctx, winners, rewardAmount)

		return types.ConsensusResult{
			TaskID:      submission.TaskID,
			ResultHash:  majorityHash,
			Workers:     winners,
			TotalCount:  len(submissions),
			QuorumCount: quorumCount,
			Achieved:    true,
		}, nil
	}

	// Quorum reached but no majority agreement yet (split vote)
	return types.ConsensusResult{
		TaskID:      submission.TaskID,
		Achieved:    false,
		TotalCount:  len(submissions),
		QuorumCount: quorumCount,
	}, nil
}
