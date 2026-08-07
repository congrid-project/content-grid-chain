package registry

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
)

func marshalSlot(s Slot) ([]byte, error) {
	return json.Marshal(s)
}

func unmarshalSlot(b []byte) (Slot, error) {
	var s Slot
	return s, json.Unmarshal(b, &s)
}

func marshalLease(l SlotLease) ([]byte, error) {
	return json.Marshal(l)
}

func unmarshalLease(b []byte) (SlotLease, error) {
	var l SlotLease
	return l, json.Unmarshal(b, &l)
}

func (k Keeper) nextSequence(ctx sdk.Context, key []byte) uint64 {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), slotMetaPrefix)
	bz := store.Get(key)
	var seq uint64
	if len(bz) > 0 {
		seq = binary.BigEndian.Uint64(bz)
	}
	seq++
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, seq)
	store.Set(key, out)
	return seq
}

func (k Keeper) nextSlotID(ctx sdk.Context) string {
	seq := k.nextSequence(ctx, slotSeqKey)
	return fmt.Sprintf("slot-%06d", seq)
}

func (k Keeper) nextLeaseID(ctx sdk.Context) string {
	seq := k.nextSequence(ctx, leaseSeqKey)
	return fmt.Sprintf("lease-%06d", seq)
}

func (k Keeper) SetSlot(ctx sdk.Context, slot Slot) error {
	slot.Domain = NormalizeDomain(slot.Domain)
	if err := slot.ValidateBasic(); err != nil {
		return err
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), slotStorePrefix)
	bz, err := marshalSlot(slot)
	if err != nil {
		return err
	}
	store.Set([]byte(slot.ID), bz)
	return nil
}

func (k Keeper) GetSlot(ctx sdk.Context, id string) (Slot, bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), slotStorePrefix)
	bz := store.Get([]byte(id))
	if len(bz) == 0 {
		return Slot{}, false
	}
	slot, err := unmarshalSlot(bz)
	if err != nil {
		panic(fmt.Errorf("failed to decode slot: %w", err))
	}
	return slot, true
}

func (k Keeper) IterateSlots(ctx sdk.Context, cb func(Slot) bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), slotStorePrefix)
	iter := store.Iterator(nil, nil)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		slot, err := unmarshalSlot(iter.Value())
		if err != nil {
			panic(fmt.Errorf("failed to decode slot: %w", err))
		}
		if stop := cb(slot); stop {
			return
		}
	}
}

func (k Keeper) transferSlotsForDomain(ctx sdk.Context, domain, previousOwner, newOwner string, nowUnix int64) error {
	domain = NormalizeDomain(domain)
	previousOwner = strings.TrimSpace(previousOwner)
	newOwner = strings.TrimSpace(newOwner)
	toTransfer := make([]Slot, 0)
	k.IterateSlots(ctx, func(slot Slot) bool {
		if slot.Domain == domain && slot.Publisher == previousOwner {
			toTransfer = append(toTransfer, slot)
		}
		return false
	})
	for _, slot := range toTransfer {
		slot.Publisher = newOwner
		slot.UpdatedAtUnix = nowUnix
		if err := k.SetSlot(ctx, slot); err != nil {
			return err
		}
	}
	return nil
}

func (k Keeper) ListSlotsPaginated(ctx sdk.Context, publisher string, status SlotStatus, pageReq *sdkquery.PageRequest) ([]Slot, *sdkquery.PageResponse, error) {
	publisher = strings.TrimSpace(publisher)
	store := prefix.NewStore(ctx.KVStore(k.storeKey), slotStorePrefix)
	var slots []Slot

	pageRes, err := sdkquery.FilteredPaginate(store, pageReq, func(_ []byte, value []byte, accumulate bool) (bool, error) {
		slot, err := unmarshalSlot(value)
		if err != nil {
			return false, err
		}
		if publisher != "" && slot.Publisher != publisher {
			return false, nil
		}
		if status != SlotStatusUnspecified && slot.Status != status {
			return false, nil
		}
		if accumulate {
			slots = append(slots, slot)
		}
		return true, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return slots, pageRes, nil
}

func (k Keeper) SetLease(ctx sdk.Context, lease SlotLease) error {
	if err := lease.ValidateBasic(); err != nil {
		return err
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), leaseStorePrefix)
	bz, err := marshalLease(lease)
	if err != nil {
		return err
	}
	store.Set([]byte(lease.ID), bz)
	return nil
}

func (k Keeper) GetLease(ctx sdk.Context, id string) (SlotLease, bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), leaseStorePrefix)
	bz := store.Get([]byte(id))
	if len(bz) == 0 {
		return SlotLease{}, false
	}
	lease, err := unmarshalLease(bz)
	if err != nil {
		panic(fmt.Errorf("failed to decode lease: %w", err))
	}
	return lease, true
}

func (k Keeper) IterateLeases(ctx sdk.Context, cb func(SlotLease) bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), leaseStorePrefix)
	iter := store.Iterator(nil, nil)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		lease, err := unmarshalLease(iter.Value())
		if err != nil {
			panic(fmt.Errorf("failed to decode lease: %w", err))
		}
		if stop := cb(lease); stop {
			return
		}
	}
}

func (k Keeper) ListLeasesPaginated(ctx sdk.Context, publisher, slotID string, activeOnly bool, atUnix int64, pageReq *sdkquery.PageRequest) ([]SlotLease, *sdkquery.PageResponse, error) {
	publisher = strings.TrimSpace(publisher)
	slotID = strings.TrimSpace(slotID)
	if atUnix <= 0 {
		atUnix = time.Now().UTC().Unix()
	}

	store := prefix.NewStore(ctx.KVStore(k.storeKey), leaseStorePrefix)
	var leases []SlotLease

	pageRes, err := sdkquery.FilteredPaginate(store, pageReq, func(_ []byte, value []byte, accumulate bool) (bool, error) {
		lease, err := unmarshalLease(value)
		if err != nil {
			return false, err
		}
		if publisher != "" && lease.Publisher != publisher {
			return false, nil
		}
		if slotID != "" && lease.SlotID != slotID {
			return false, nil
		}
		if activeOnly {
			if lease.Status != LeaseStatusActive {
				return false, nil
			}
			if atUnix < lease.StartsAtUnix || atUnix > lease.EndsAtUnix {
				return false, nil
			}
		}
		if accumulate {
			leases = append(leases, lease)
		}
		return true, nil
	})
	if err != nil {
		return nil, nil, err
	}
	return leases, pageRes, nil
}

func (k Keeper) listLeasesBySlot(ctx sdk.Context, slotID string) []SlotLease {
	var out []SlotLease
	k.IterateLeases(ctx, func(l SlotLease) bool {
		if l.SlotID == slotID {
			out = append(out, l)
		}
		return false
	})
	return out
}
