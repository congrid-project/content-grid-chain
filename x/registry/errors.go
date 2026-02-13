package registry

import errorsmod "cosmossdk.io/errors"

var (
	ErrInvalidPublisherRequest = errorsmod.Register(ModuleName, 1, "invalid publisher request")
	ErrPublisherNotFound       = errorsmod.Register(ModuleName, 2, "publisher not found")
	ErrAssignmentNotFound      = errorsmod.Register(ModuleName, 3, "verification assignment not found")
	ErrVerifierNotAssigned     = errorsmod.Register(ModuleName, 4, "verifier not assigned")
	ErrAssignmentClosed        = errorsmod.Register(ModuleName, 5, "verification window closed")
	ErrAlreadySubmitted        = errorsmod.Register(ModuleName, 6, "verification already submitted")
	ErrAlreadyCommitted        = errorsmod.Register(ModuleName, 7, "verification commit already submitted")
	ErrCommitWindowClosed      = errorsmod.Register(ModuleName, 8, "verification commit window closed")
	ErrRevealWindowNotOpen     = errorsmod.Register(ModuleName, 9, "verification reveal window not open")
	ErrCommitNotFound          = errorsmod.Register(ModuleName, 10, "verification commit not found")
	ErrCommitMismatch          = errorsmod.Register(ModuleName, 11, "verification commit hash mismatch")
	ErrSlotNotFound            = errorsmod.Register(ModuleName, 12, "slot not found")
	ErrSlotNotListed           = errorsmod.Register(ModuleName, 13, "slot not listed")
	ErrSlotUnauthorized        = errorsmod.Register(ModuleName, 14, "slot not owned by publisher")
	ErrLeaseOverlap            = errorsmod.Register(ModuleName, 15, "lease overlaps existing lease")
	ErrLeaseInvalidDuration    = errorsmod.Register(ModuleName, 16, "lease duration invalid")
	ErrLeasePaymentFailed      = errorsmod.Register(ModuleName, 17, "lease escrow payment failed")
	ErrPublisherInCooldown     = errorsmod.Register(ModuleName, 18, "publisher in cooldown")
)
