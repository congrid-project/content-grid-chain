package miners

import errorsmod "cosmossdk.io/errors"

var (
	ErrMinerExists        = errorsmod.Register(ModuleName, 1, "miner already exists")
	ErrMinerNotFound      = errorsmod.Register(ModuleName, 2, "miner not found")
	ErrInvalidMinerUpdate = errorsmod.Register(ModuleName, 3, "invalid miner update")
)
