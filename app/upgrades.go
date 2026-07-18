package app

import (
	"context"
	"fmt"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	"content-grid-chain/x/nodes"
	"content-grid-chain/x/registry"
	"content-grid-chain/x/tokenomics"
	"content-grid-chain/x/verifiers"
)

// DrandStrictV2UpgradeName is the on-chain software-upgrade plan name. The
// governance proposal must use this exact value.
const DrandStrictV2UpgradeName = "drand-strict-v2"

func registerUpgradeHandlers(app *App) {
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
