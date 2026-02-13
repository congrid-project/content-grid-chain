package keeper

import (
	"content-grid-chain/x/tasks/types"
	"encoding/json"

	sdkmath "cosmossdk.io/math"
	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	AssignmentStorePrefix = []byte{0x01}
	SubmissionStorePrefix = []byte{0x02}
	ParamsStoreKey        = []byte{0x03}
)

// TokenomicsKeeper defines the interface for rewarding miners.
type TokenomicsKeeper interface {
	DistributeTaskRewards(ctx sdk.Context, winners []string, totalAmount sdkmath.Int) error
}

// Keeper holds in-memory helpers and store keys for task management.
type Keeper struct {
	cdc            codec.BinaryCodec
	storeKey       storetypes.StoreKey
	tokenomicsKeep TokenomicsKeeper
}

// NewKeeper constructs a keeper with validated params.
func NewKeeper(cdc codec.BinaryCodec, storeKey storetypes.StoreKey, tk TokenomicsKeeper) Keeper {
	return Keeper{
		cdc:            cdc,
		storeKey:       storeKey,
		tokenomicsKeep: tk,
	}
}

// SetParams stores the active parameters.
func (k Keeper) SetParams(ctx sdk.Context, params types.Params) error {
	if err := params.Validate(); err != nil {
		return err
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), ParamsStoreKey)
	bz, err := json.Marshal(params)
	if err != nil {
		return err
	}
	store.Set([]byte{0x00}, bz)
	return nil
}

// GetParams fetches the active params, defaulting if unset.
func (k Keeper) Params(ctx sdk.Context) types.Params {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), ParamsStoreKey)
	bz := store.Get([]byte{0x00})
	if len(bz) == 0 {
		return types.DefaultParams()
	}
	var params types.Params
	if err := json.Unmarshal(bz, &params); err != nil {
		// Fallback to default if corrupted (should not happen)
		return types.DefaultParams()
	}
	return params
}

// SetAssignment stores a task assignment.
func (k Keeper) SetAssignment(ctx sdk.Context, assignment types.Assignment) error {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), AssignmentStorePrefix)
	bz, err := json.Marshal(assignment)
	if err != nil {
		return err
	}
	store.Set([]byte(assignment.TaskID), bz)
	return nil
}

// GetAssignment retrieves a task assignment.
func (k Keeper) GetAssignment(ctx sdk.Context, taskID string) (types.Assignment, bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), AssignmentStorePrefix)
	bz := store.Get([]byte(taskID))
	if len(bz) == 0 {
		return types.Assignment{}, false
	}
	var assignment types.Assignment
	if err := json.Unmarshal(bz, &assignment); err != nil {
		return types.Assignment{}, false
	}
	return assignment, true
}

// SetSubmissions stores the list of submissions for a task.
func (k Keeper) SetSubmissions(ctx sdk.Context, taskID string, submissions []types.TaskSubmission) error {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), SubmissionStorePrefix)
	bz, err := json.Marshal(submissions)
	if err != nil {
		return err
	}
	store.Set([]byte(taskID), bz)
	return nil
}

// GetSubmissions retrieves the list of submissions for a task.
func (k Keeper) GetSubmissions(ctx sdk.Context, taskID string) []types.TaskSubmission {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), SubmissionStorePrefix)
	bz := store.Get([]byte(taskID))
	if len(bz) == 0 {
		return []types.TaskSubmission{}
	}
	var submissions []types.TaskSubmission
	if err := json.Unmarshal(bz, &submissions); err != nil {
		return []types.TaskSubmission{}
	}
	return submissions
}
