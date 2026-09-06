package app

import (
	"context"
	"fmt"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	transfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
	ibcexported "github.com/cosmos/ibc-go/v10/modules/core/exported"
	ibctm "github.com/cosmos/ibc-go/v10/modules/light-clients/07-tendermint"

	"content-grid-chain/x/nodes"
	"content-grid-chain/x/registry"
	"content-grid-chain/x/tokenomics"
	"content-grid-chain/x/verifiers"
)

// DrandStrictV2UpgradeName is the on-chain software-upgrade plan name. The
// governance proposal must use this exact value.
const DrandStrictV2UpgradeName = "drand-strict-v2"

// PublisherRewardsV3UpgradeName enables round-level publisher rewards, the
// ten-percent minimum payout, and verifier-confirmed publisher re-registration.
const PublisherRewardsV3UpgradeName = "publisher-rewards-v3"

// IBCTransferV1UpgradeName adds IBC Core and ICS-20 to an existing chain.
const IBCTransferV1UpgradeName = "ibc-transfer-v1"

func registerUpgradeHandlers(app *App) {
	app.UpgradeKeeper.SetUpgradeHandler(IBCTransferV1UpgradeName, func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		// Require the current pre-IBC release as the baseline. Missing historical
		// module versions must never cause RunMigrations to reset live state.
		for name, version := range app.ModuleManager.GetVersionMap() {
			if name == ibcexported.ModuleName || name == transfertypes.ModuleName || name == ibctm.ModuleName {
				if _, exists := fromVM[name]; exists {
					return nil, fmt.Errorf("%s already exists before %s", name, plan.Name)
				}
				continue
			}
			if old, exists := fromVM[name]; !exists || old != version {
				return nil, fmt.Errorf("%s requires the current pre-IBC baseline: module %s version %d (recorded %d, present %t)", plan.Name, name, version, old, exists)
			}
		}
		// Absent IBC/transfer entries intentionally trigger InitGenesis, creating
		// client/connection params, the transfer port and its module account.
		vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
		if err != nil {
			return nil, fmt.Errorf("initialize IBC for %s: %w", plan.Name, err)
		}
		return vm, nil
	})
	app.UpgradeKeeper.SetUpgradeHandler(
		DrandStrictV2UpgradeName,
		func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			sdkCtx.Logger().Info("running software upgrade", "name", plan.Name, "height", plan.Height)

			fromVM = prepareDrandStrictV2VersionMap(fromVM, app.ModuleManager.GetVersionMap())
			updatedVM, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, fmt.Errorf("run module migrations for %s: %w", DrandStrictV2UpgradeName, err)
			}

			params := app.RegistryKeeper.GetParams(sdkCtx)
			params = params.WithStrictDrandEnabled()
			if err := app.RegistryKeeper.SetParams(sdkCtx, params); err != nil {
				return nil, fmt.Errorf("enable strict drand for %s: %w", DrandStrictV2UpgradeName, err)
			}
			return updatedVM, nil
		},
	)
	app.UpgradeKeeper.SetUpgradeHandler(
		PublisherRewardsV3UpgradeName,
		func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			sdkCtx.Logger().Info("running software upgrade", "name", plan.Name, "height", plan.Height)

			fromVM = preparePublisherRewardsV3VersionMap(fromVM, app.ModuleManager.GetVersionMap())
			updatedVM, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, fmt.Errorf("run module migrations for %s: %w", PublisherRewardsV3UpgradeName, err)
			}
			return updatedVM, nil
		},
	)
}

// configureIBCStoreLoader must run before loading the database. The old binary
// writes upgrade-info.json when it halts; stores are added only at that height.
func configureIBCStoreLoader(app *App) error {
	plan, err := app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	if err != nil {
		return fmt.Errorf("read IBC upgrade info: %w", err)
	}
	if plan.Name == IBCTransferV1UpgradeName && !app.UpgradeKeeper.IsSkipHeight(plan.Height) {
		app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(plan.Height, &storetypes.StoreUpgrades{
			Added: []string{ibcexported.StoreKey, transfertypes.StoreKey},
		}))
	}
	return nil
}

func preparePublisherRewardsV3VersionMap(fromVM, targetVM module.VersionMap) module.VersionMap {
	out := make(module.VersionMap, len(fromVM)+4)
	for name, version := range fromVM {
		out[name] = version
	}
	for _, name := range []string{nodes.ModuleName, verifiers.ModuleName, tokenomics.ModuleName} {
		if _, found := out[name]; !found {
			out[name] = targetVM[name]
		}
	}
	if _, found := out[registry.ModuleName]; !found {
		out[registry.ModuleName] = 2
	}
	return out
}

// prepareDrandStrictV2VersionMap protects chains created before the custom
// modules were included in x/upgrade's initial VersionMap. Registry existed at
// consensus version 1 on those chains; the other custom modules are unchanged.
func prepareDrandStrictV2VersionMap(fromVM, targetVM module.VersionMap) module.VersionMap {
	out := make(module.VersionMap, len(fromVM)+4)
	for name, version := range fromVM {
		out[name] = version
	}
	for _, name := range []string{nodes.ModuleName, verifiers.ModuleName, tokenomics.ModuleName} {
		if _, found := out[name]; !found {
			out[name] = targetVM[name]
		}
	}
	if _, found := out[registry.ModuleName]; !found {
		out[registry.ModuleName] = 1
	}
	return out
}
