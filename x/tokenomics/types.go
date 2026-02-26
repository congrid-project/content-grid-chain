package tokenomics

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
)

// DefaultDenom is the canonical micro-denomination used chain-wide.
const DefaultDenom = "ucongrid"

// DefaultInitialSupply sets an illustrative initial supply (1B CONGRID with 6 decimals).
var DefaultInitialSupply = sdkmath.NewInt(1_000_000_000_000000)

// VestingSchedule captures cliff and vesting periods in months.
type VestingSchedule struct {
	CliffMonths   int `json:"cliff_months"`
	VestingMonths int `json:"vesting_months"`
}

// GenesisAllocation describes how a category receives its share of genesis tokens.
type GenesisAllocation struct {
	Category string            `json:"category"`
	Percent  sdkmath.LegacyDec `json:"percent"`
	Address  string            `json:"address"`
	Notes    string            `json:"notes"`
	Vesting  VestingSchedule   `json:"vesting"`
}

// Validate ensures the allocation entry is self-consistent.
func (ga GenesisAllocation) Validate() error {
	if ga.Category == "" {
		return fmt.Errorf("allocation category required")
	}
	if err := ensureUnitInterval(ga.Percent, ga.Category+" percent"); err != nil {
		return err
	}
	if ga.Vesting.CliffMonths < 0 {
		return fmt.Errorf("cliff months must be >= 0")
	}
	if ga.Vesting.VestingMonths < 0 {
		return fmt.Errorf("vesting months must be >= 0")
	}
	if ga.Vesting.VestingMonths > 0 && ga.Vesting.CliffMonths > ga.Vesting.VestingMonths {
		return fmt.Errorf("cliff months (%d) must be <= vesting months (%d)", ga.Vesting.CliffMonths, ga.Vesting.VestingMonths)
	}
	return nil
}

// GenesisState wires the default monetary policy for genesis helpers.
type GenesisState struct {
	Denom         string              `json:"denom"`
	InitialSupply sdkmath.Int         `json:"initial_supply"`
	Allocations   []GenesisAllocation `json:"allocations"`
	Params        Params              `json:"params"`
}

// DefaultGenesisState returns the default configuration for this module.
func DefaultGenesisState() GenesisState {
	return GenesisState{
		Denom:         DefaultDenom,
		InitialSupply: DefaultInitialSupply,
		Allocations: []GenesisAllocation{
			{
				Category: "foundation_reserve",
				Percent:  mustNewDec("0.25"),
				Address:  "grid1foundationplaceholder0000000000000000000000",
				Notes:    "4-year linear release via multisig treasury",
				Vesting: VestingSchedule{
					CliffMonths:   0,
					VestingMonths: 48,
				},
			},
			{
				Category: "team_and_advisors",
				Percent:  mustNewDec("0.20"),
				Address:  "grid1teamplaceholder000000000000000000000000",
				Notes:    "12-month cliff, 36-month vesting",
				Vesting: VestingSchedule{
					CliffMonths:   12,
					VestingMonths: 48,
				},
			},
			{
				Category: "verifier_bootstrap_pool",
				Percent:  mustNewDec("0.20"),
				Address:  "grid1verifierbootstrap000000000000000000",
				Notes:    "delegated incentives for early verifier operators",
				Vesting: VestingSchedule{
					CliffMonths:   0,
					VestingMonths: 24,
				},
			},
			{
				Category: "publisher_growth_fund",
				Percent:  mustNewDec("0.15"),
				Address:  "grid1publisherfund0000000000000000000000",
				Notes:    "milestone-based rebates for verified sites",
				Vesting: VestingSchedule{
					CliffMonths:   0,
					VestingMonths: 36,
				},
			},
			{
				Category: "public_sale_and_liquidity",
				Percent:  mustNewDec("0.10"),
				Address:  "grid1liquiditypool000000000000000000000",
				Notes:    "market making and launch pool",
				Vesting: VestingSchedule{
					CliffMonths:   0,
					VestingMonths: 12,
				},
			},
			{
				Category: "community_treasury",
				Percent:  mustNewDec("0.10"),
				Address:  "grid1communitydao000000000000000000000",
				Notes:    "DAO-controlled discretionary budget",
				Vesting: VestingSchedule{
					CliffMonths:   0,
					VestingMonths: 0,
				},
			},
		},
		Params: DefaultParams(),
	}
}

// Validate checks the genesis state for correctness.
func (gs GenesisState) Validate() error {
	if gs.Denom == "" {
		return fmt.Errorf("denom required")
	}
	if !gs.InitialSupply.IsPositive() {
		return fmt.Errorf("initial supply must be positive")
	}
	if len(gs.Allocations) == 0 {
		return fmt.Errorf("at least one allocation required")
	}
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("invalid params: %w", err)
	}

	seenCategories := make(map[string]struct{})
	total := sdkmath.LegacyZeroDec()
	for _, alloc := range gs.Allocations {
		if err := alloc.Validate(); err != nil {
			return fmt.Errorf("allocation %s invalid: %w", alloc.Category, err)
		}
		if _, exists := seenCategories[alloc.Category]; exists {
			return fmt.Errorf("duplicate allocation category: %s", alloc.Category)
		}
		seenCategories[alloc.Category] = struct{}{}
		total = total.Add(alloc.Percent)
	}
	if err := ensureSharesSumToOne([]sdkmath.LegacyDec{total}, "genesis allocations"); err != nil {
		return fmt.Errorf("allocations must sum to 1: %w", err)
	}
	return nil
}

// AllocationBreakdown converts percentage-based allocations to integer coin amounts.
func (gs GenesisState) AllocationBreakdown() (map[string]sdkmath.Int, error) {
	if err := gs.Validate(); err != nil {
		return nil, err
	}
	breakdown := make(map[string]sdkmath.Int, len(gs.Allocations))
	remaining := gs.InitialSupply
	for i, alloc := range gs.Allocations {
		share := alloc.Percent.MulInt(gs.InitialSupply).TruncateInt()
		if i == len(gs.Allocations)-1 {
			share = remaining
		} else {
			remaining = remaining.Sub(share)
		}
		breakdown[alloc.Category] = share
	}
	return breakdown, nil
}

// WithAllocationAddress returns a copy of the genesis state with a specific category updated.
func (gs GenesisState) WithAllocationAddress(category, address string) GenesisState {
	updated := gs
	for i, alloc := range updated.Allocations {
		if alloc.Category == category {
			alloc.Address = address
			updated.Allocations[i] = alloc
			break
		}
	}
	return updated
}
