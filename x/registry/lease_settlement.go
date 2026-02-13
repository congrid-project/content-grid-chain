package registry

import (
	"fmt"
	"strings"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (k Keeper) settleLeases(ctx sdk.Context) error {
	nowUnix := ctx.BlockTime().UTC().Unix()
	var errOut error

	k.IterateLeases(ctx, func(lease SlotLease) bool {
		if lease.Status != LeaseStatusActive {
			return false
		}
		if nowUnix < lease.EndsAtUnix {
			return false
		}
		slot, found := k.GetSlot(ctx, lease.SlotID)
		if !found {
			errOut = fmt.Errorf("slot %s not found for lease %s", lease.SlotID, lease.ID)
			return true
		}
		website, found := k.GetWebsite(ctx, slot.Domain)
		if !found {
			errOut = fmt.Errorf("publisher %s not found for lease %s", slot.Domain, lease.ID)
			return true
		}
		if website.CooldownUntilUnix > nowUnix {
			if err := k.refundLease(ctx, lease, LeaseStatusViolated, nowUnix); err != nil {
				errOut = err
				return true
			}
			return false
		}
		if err := k.payoutLease(ctx, lease, nowUnix); err != nil {
			errOut = err
			return true
		}
		return false
	})

	return errOut
}

func (k Keeper) payoutLease(ctx sdk.Context, lease SlotLease, nowUnix int64) error {
	payout := lease.EscrowRemaining
	if payout.IsPositive() {
		publisherAddr, err := sdk.AccAddressFromBech32(lease.Publisher)
		if err != nil {
			return err
		}
		coin := sdk.NewCoin(lease.RateDenom, payout)
		if err := k.bank.SendCoinsFromModuleToAccount(ctx, ModuleName, publisherAddr, sdk.NewCoins(coin)); err != nil {
			return err
		}
	}

	lease.PaidOut = lease.PaidOut.Add(payout)
	lease.EscrowRemaining = sdkmath.ZeroInt()
	lease.Status = LeaseStatusCompleted
	lease.UpdatedAtUnix = nowUnix
	lease.PaidThroughUnix = lease.EndsAtUnix
	if err := k.SetLease(ctx, lease); err != nil {
		return err
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			EventTypeLeaseSettled,
			sdk.NewAttribute(AttributeKeyLeaseID, lease.ID),
			sdk.NewAttribute(AttributeKeySlotID, lease.SlotID),
			sdk.NewAttribute(AttributeKeyOwner, lease.Publisher),
			sdk.NewAttribute(AttributeKeyLeaseStatus, lease.Status.String()),
			sdk.NewAttribute(AttributeKeyPayoutAmount, payout.String()),
		),
	)

	return nil
}

func (k Keeper) refundLease(ctx sdk.Context, lease SlotLease, status LeaseStatus, nowUnix int64) error {
	refund := lease.EscrowRemaining
	if refund.IsPositive() {
		lesseeAddr, err := sdk.AccAddressFromBech32(lease.Lessee)
		if err != nil {
			return err
		}
		coin := sdk.NewCoin(lease.RateDenom, refund)
		if err := k.bank.SendCoinsFromModuleToAccount(ctx, ModuleName, lesseeAddr, sdk.NewCoins(coin)); err != nil {
			return err
		}
	}

	lease.EscrowRemaining = sdkmath.ZeroInt()
	lease.Status = status
	lease.UpdatedAtUnix = nowUnix
	lease.PaidThroughUnix = nowUnix
	if err := k.SetLease(ctx, lease); err != nil {
		return err
	}

	eventType := EventTypeLeaseSettled
	if status == LeaseStatusViolated {
		eventType = EventTypeLeaseViolated
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			eventType,
			sdk.NewAttribute(AttributeKeyLeaseID, lease.ID),
			sdk.NewAttribute(AttributeKeySlotID, lease.SlotID),
			sdk.NewAttribute(AttributeKeyOwner, lease.Publisher),
			sdk.NewAttribute(AttributeKeyLeaseStatus, lease.Status.String()),
			sdk.NewAttribute(AttributeKeyPayoutAmount, refund.String()),
		),
	)

	return nil
}

func (k Keeper) activeLeasesForPublisher(ctx sdk.Context, publisher string, atUnix int64) []SlotLease {
	publisher = strings.TrimSpace(publisher)
	if atUnix <= 0 {
		atUnix = ctx.BlockTime().UTC().Unix()
	}
	var out []SlotLease
	k.IterateLeases(ctx, func(lease SlotLease) bool {
		if lease.Status != LeaseStatusActive {
			return false
		}
		if publisher != "" && lease.Publisher != publisher {
			return false
		}
		if atUnix < lease.StartsAtUnix || atUnix > lease.EndsAtUnix {
			return false
		}
		out = append(out, lease)
		return false
	})
	return out
}

func (k Keeper) activeLeasesForDomain(ctx sdk.Context, domain string, atUnix int64) []SlotLease {
	domain = NormalizeDomain(domain)
	if atUnix <= 0 {
		atUnix = ctx.BlockTime().UTC().Unix()
	}
	var out []SlotLease
	k.IterateLeases(ctx, func(lease SlotLease) bool {
		if lease.Status != LeaseStatusActive {
			return false
		}
		if atUnix < lease.StartsAtUnix || atUnix > lease.EndsAtUnix {
			return false
		}
		slot, found := k.GetSlot(ctx, lease.SlotID)
		if !found {
			return false
		}
		if NormalizeDomain(slot.Domain) != domain {
			return false
		}
		out = append(out, lease)
		return false
	})
	return out
}

func computeCooldownSeconds(baseSeconds int64, count int32) int64 {
	if baseSeconds <= 0 || count <= 0 {
		return 0
	}
	duration := baseSeconds
	for i := int32(1); i < count; i++ {
		duration *= 2
	}
	return duration
}

func (k Keeper) applyPublisherCooldown(ctx sdk.Context, website Website, params PublisherParams, nowUnix int64) (Website, int64, error) {
	count := website.CooldownCount + 1
	duration := computeCooldownSeconds(params.CooldownBaseSeconds, count)
	if duration <= 0 {
		return website, 0, fmt.Errorf("cooldown duration invalid")
	}
	website.CooldownCount = count
	website.CooldownUntilUnix = nowUnix + duration
	if err := k.UpsertWebsite(ctx, website); err != nil {
		return website, 0, err
	}
	return website, duration, nil
}

func (k Keeper) clearPublisherCooldown(ctx sdk.Context, website Website) (Website, bool, error) {
	if website.CooldownUntilUnix == 0 && website.CooldownCount == 0 {
		return website, false, nil
	}
	website.CooldownUntilUnix = 0
	website.CooldownCount = 0
	if err := k.UpsertWebsite(ctx, website); err != nil {
		return website, false, err
	}
	return website, true, nil
}

func (k Keeper) violateActiveLeases(ctx sdk.Context, publisher string, nowUnix int64) error {
	leases := k.activeLeasesForPublisher(ctx, publisher, nowUnix)
	for _, lease := range leases {
		if err := k.refundLease(ctx, lease, LeaseStatusViolated, nowUnix); err != nil {
			return err
		}
	}
	return nil
}

func (k Keeper) violateActiveLeasesForDomain(ctx sdk.Context, domain string, nowUnix int64) error {
	leases := k.activeLeasesForDomain(ctx, domain, nowUnix)
	for _, lease := range leases {
		if err := k.refundLease(ctx, lease, LeaseStatusViolated, nowUnix); err != nil {
			return err
		}
	}
	return nil
}
