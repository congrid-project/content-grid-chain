package registry

import (
	"encoding/json"
	"errors"
	"fmt"

	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkquery "github.com/cosmos/cosmos-sdk/types/query"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
)

var (
	websiteStorePrefix        = []byte{0x01}
	paramsStoreKey            = []byte{0x02}
	primaryDomainStorePrefix  = []byte{0x03}
	assignmentStorePrefix     = []byte{0x10}
	submissionStorePrefix     = []byte{0x11}
	verificationMetaPrefix    = []byte{0x12}
	commitStorePrefix         = []byte{0x13}
	publisherFailStorePrefix  = []byte{0x14}
	verifierPenaltyPrefix     = []byte{0x15}
	drandStorePrefix          = []byte{0x16}
	unsettledRoundStorePrefix = []byte{0x17}
	slotStorePrefix           = []byte{0x20}
	leaseStorePrefix          = []byte{0x21}
	slotMetaPrefix            = []byte{0x22}
	slotSeqKey                = []byte{0x01}
	leaseSeqKey               = []byte{0x02}
	lastRoundStartKey         = []byte{0x00}
	roundMetaKeyPrefix        = []byte{0x01}
	drandLatestRoundKey       = []byte{0x02}
	drandBeaconKeyPrefix      = []byte{0x03}

	// ErrWebsiteExists is returned when attempting to register an already tracked domain.
	ErrWebsiteExists = errors.New("website already registered")
)

// Keeper persists publisher registrations and their params.
type Keeper struct {
	cdc        codec.BinaryCodec
	storeKey   storetypes.StoreKey
	verifiers  VerifierKeeper
	tokenomics TokenomicsKeeper
	bank       bankkeeper.Keeper
}

// NewKeeper instantiates the registry keeper.
func NewKeeper(cdc codec.BinaryCodec, storeKey storetypes.StoreKey, verifiersKeeper VerifierKeeper, tokenomicsKeeper TokenomicsKeeper, bankKeeper bankkeeper.Keeper) Keeper {
	return Keeper{
		cdc:        cdc,
		storeKey:   storeKey,
		verifiers:  verifiersKeeper,
		tokenomics: tokenomicsKeeper,
		bank:       bankKeeper,
	}
}

// SetParams stores the active publisher parameters.
func (k Keeper) SetParams(ctx sdk.Context, params PublisherParams) error {
	if err := params.Validate(); err != nil {
		return err
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), paramsStoreKey)
	bz, err := json.Marshal(params)
	if err != nil {
		return err
	}
	store.Set([]byte{0x00}, bz)
	return nil
}

// GetParams fetches the active params, defaulting if unset.
func (k Keeper) GetParams(ctx sdk.Context) PublisherParams {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), paramsStoreKey)
	bz := store.Get([]byte{0x00})
	if len(bz) == 0 {
		return DefaultPublisherParams()
	}
	var params PublisherParams
	if err := json.Unmarshal(bz, &params); err != nil {
		panic(fmt.Errorf("failed to decode registry params: %w", err))
	}
	return params
}

// RegisterWebsite persists a new website entry if it does not already exist.
func (k Keeper) RegisterWebsite(ctx sdk.Context, website Website) (Website, error) {
	website.Domain = NormalizeDomain(website.Domain)
	if website.Status == StatusUnspecified {
		website.Status = StatusPending
	}
	website.RegisteredAtHeight = ctx.BlockHeight()

	if err := ValidateWebsite(website); err != nil {
		return Website{}, err
	}

	store := prefix.NewStore(ctx.KVStore(k.storeKey), websiteStorePrefix)
	key := []byte(website.Domain)
	if store.Has(key) {
		return Website{}, fmt.Errorf("%w: %s", ErrWebsiteExists, website.Domain)
	}

	// Enforce primary domain uniqueness
	primary, err := GetPrimaryDomain(website.Domain)
	if err != nil {
		return Website{}, fmt.Errorf("failed to extract primary domain: %w", err)
	}
	pStore := prefix.NewStore(ctx.KVStore(k.storeKey), primaryDomainStorePrefix)
	pKey := []byte(primary)
	if pStore.Has(pKey) {
		existing := string(pStore.Get(pKey))
		return Website{}, fmt.Errorf("primary domain %s already registered by %s", primary, existing)
	}

	bz, err := marshalWebsite(website)
	if err != nil {
		return Website{}, err
	}
	store.Set(key, bz)
	pStore.Set(pKey, []byte(website.Domain))
	return website, nil
}

// UpsertWebsite overwrites an existing entry after validation.
func (k Keeper) UpsertWebsite(ctx sdk.Context, website Website) error {
	website.Domain = NormalizeDomain(website.Domain)
	if err := ValidateWebsite(website); err != nil {
		return err
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), websiteStorePrefix)
	bz, err := marshalWebsite(website)
	if err != nil {
		return err
	}
	store.Set([]byte(website.Domain), bz)
	return nil
}

// GetWebsite retrieves a website if it exists.
func (k Keeper) GetWebsite(ctx sdk.Context, domain string) (Website, bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), websiteStorePrefix)
	bz := store.Get([]byte(NormalizeDomain(domain)))
	if len(bz) == 0 {
		return Website{}, false
	}
	website, err := unmarshalWebsite(bz)
	if err != nil {
		panic(fmt.Errorf("failed to decode website %s: %w", domain, err))
	}
	return website, true
}

// IterateWebsites walks all stored websites.
func (k Keeper) IterateWebsites(ctx sdk.Context, cb func(Website) bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), websiteStorePrefix)
	iterator := store.Iterator(nil, nil)
	defer iterator.Close()

	for ; iterator.Valid(); iterator.Next() {
		website, err := unmarshalWebsite(iterator.Value())
		if err != nil {
			panic(fmt.Errorf("failed to decode website: %w", err))
		}
		if stop := cb(website); stop {
			return
		}
	}
}

// GetWebsitesPaginated returns the stored websites with pagination support.
func (k Keeper) GetWebsitesPaginated(ctx sdk.Context, pageReq *sdkquery.PageRequest) ([]Website, *sdkquery.PageResponse, error) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), websiteStorePrefix)
	var websites []Website

	pageRes, err := sdkquery.Paginate(store, pageReq, func(_, value []byte) error {
		website, err := unmarshalWebsite(value)
		if err != nil {
			return err
		}
		websites = append(websites, website)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return websites, pageRes, nil
}
