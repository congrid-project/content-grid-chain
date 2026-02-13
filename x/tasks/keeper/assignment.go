package keeper

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"

	"content-grid-chain/x/tasks/types" // Updated import path

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// GetTaskAssignment calculates the deterministic assignment of a task to miners based on block hash.
// It uses the formula: Hash(BlockHash + TaskID) to seed the selection.
func (k Keeper) GetTaskAssignment(ctx sdk.Context, taskID string, totalMiners []string) (types.Assignment, error) { // Changed tasks.Assignment to types.Assignment
	if len(totalMiners) == 0 {
		return types.Assignment{}, fmt.Errorf("no miners available") // Changed tasks.Assignment to types.Assignment
	}

	// Get current block hash (or previous block hash if current is not available in context)
	// In Cosmos SDK, ctx.BlockHeader().LastBlockId.Hash is often used for randomness seed
	// because current block hash is not determined until Commit.
	blockHash := ctx.BlockHeader().LastBlockId.Hash
	if len(blockHash) == 0 {
		// Fallback for genesis or first block
		blockHash = []byte("genesis_seed")
	}
	blockHashHex := hex.EncodeToString(blockHash)

	// Sort miners to ensure deterministic order
	sort.Strings(totalMiners)

	// Determine how many miners to select (e.g. from Params)
	params := k.Params(ctx)
	numToSelect := params.MaxAssignments
	if numToSelect > len(totalMiners) {
		numToSelect = len(totalMiners)
	}

	selectedWorkers := make([]string, 0, numToSelect)

	// Simple selection logic:
	// We need to select k distinct miners.
	// We can hash (Seed + Counter) to get an index.

	seed := append(blockHash, []byte(taskID)...)

	// Create a map to track selected indices to avoid duplicates
	selectedIndices := make(map[int]struct{})

	for i := 0; i < numToSelect; i++ {
		// Generate a hash for this selection round
		roundSeed := append(seed, byte(i))
		hash := sha256.Sum256(roundSeed)

		// Convert first 8 bytes to uint64 for modulo
		val := binary.BigEndian.Uint64(hash[:8])

		// Simple rejection sampling to avoid duplicates
		// In a large set, collisions are rare. In a small set, we might loop.
		// For simplicity here, we just take the index. If collision, we increment.
		idx := int(val % uint64(len(totalMiners)))

		// Linear probe for open slot
		startIdx := idx
		for {
			if _, exists := selectedIndices[idx]; !exists {
				selectedIndices[idx] = struct{}{}
				selectedWorkers = append(selectedWorkers, totalMiners[idx])
				break
			}
			idx = (idx + 1) % len(totalMiners)
			if idx == startIdx {
				// Should not happen if numToSelect <= len(totalMiners)
				break
			}
		}
	}

	return types.Assignment{
		TaskID:      taskID,
		BlockHeight: ctx.BlockHeight(),
		BlockHash:   blockHashHex,
		Workers:     selectedWorkers,
	}, nil
}
