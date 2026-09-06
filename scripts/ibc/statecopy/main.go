//go:build ibc_rehearsal

// This offline-only harness is built against both the exact production source
// and the candidate source. It never starts networking or loads signing keys.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"content-grid-chain/app"
	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	abci "github.com/cometbft/cometbft/abci/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/spf13/viper"
)

const planName = "ibc-transfer-v1"

type rawState map[string]map[string][]byte
type digest struct {
	Keys   int    `json:"keys"`
	SHA256 string `json:"sha256"`
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func write(path string, v any) {
	b, e := json.MarshalIndent(v, "", "  ")
	must(e)
	must(os.WriteFile(path, append(b, '\n'), 0600))
}
func read(path string, v any) { b, e := os.ReadFile(path); must(e); must(json.Unmarshal(b, v)) }
func snapshot(a *app.App, ctx sdk.Context) rawState {
	out := rawState{}
	for _, key := range a.GetStoreKeys() {
		if _, ok := key.(*storetypes.KVStoreKey); !ok {
			continue
		}
		kv := map[string][]byte{}
		it := ctx.KVStore(key).Iterator(nil, nil)
		for ; it.Valid(); it.Next() {
			kv[hex.EncodeToString(it.Key())] = bytes.Clone(it.Value())
		}
		must(it.Close())
		out[key.Name()] = kv
	}
	return out
}
func digests(state rawState) map[string]digest {
	out := map[string]digest{}
	for name, kv := range state {
		keys := make([]string, 0, len(kv))
		for k := range kv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		h := sha256.New()
		for _, k := range keys {
			key, e := hex.DecodeString(k)
			must(e)
			for _, v := range [][]byte{key, kv[k]} {
				must(binary.Write(h, binary.BigEndian, uint64(len(v))))
				_, e = h.Write(v)
				must(e)
			}
		}
		out[name] = digest{len(kv), hex.EncodeToString(h.Sum(nil))}
	}
	return out
}
func stateDiff(before, after rawState) map[string][]string {
	changes := map[string][]string{}
	for name, kv := range before {
		for k, v := range kv {
			if other, exists := after[name][k]; !exists || !bytes.Equal(v, other) {
				changes[name] = append(changes[name], k)
			}
		}
		for k := range after[name] {
			if _, ok := kv[k]; !ok {
				changes[name] = append(changes[name], k)
			}
		}
	}
	for name, kv := range after {
		if _, ok := before[name]; !ok {
			for k := range kv {
				changes[name] = append(changes[name], k)
			}
		}
	}
	for _, keys := range changes {
		sort.Strings(keys)
	}
	return changes
}
func export(a *app.App, ctx sdk.Context, path string) {
	// Manual IBC modules are absent in the production build; use that build's
	// registered modules, and include IBC in the candidate after initialization.
	state, err := a.ModuleManager.ExportGenesis(ctx, a.AppCodec())
	must(err)
	write(path, state)
}
func main() {
	mode := flag.String("mode", "inspect", "inspect, prepare, migrate or verify")
	home := flag.String("home", "", "disposable copied node home")
	expected := flag.String("expected-app-hash", "", "verified production commit hash (hex)")
	at := flag.String("block-time", "", "source block time, RFC3339Nano")
	flag.Parse()
	must(run(*mode, *home, *expected, *at))
}
func run(mode, home, expected, at string) error {
	if mode != "inspect" && mode != "prepare" && mode != "migrate" && mode != "verify" {
		return fmt.Errorf("unknown mode %q", mode)
	}
	abs, err := filepath.Abs(home)
	if err != nil {
		return err
	}
	home = abs
	// This marker must be created by the operator after extracting a disposable
	// copy. Refuse symlinked stores and any copied production signing material.
	if _, err = os.Stat(filepath.Join(home, "IBC_REHEARSAL_COPY")); err != nil {
		return fmt.Errorf("missing disposable-copy marker: %w", err)
	}
	for _, name := range []string{"data", "data/application.db", "config"} {
		p := filepath.Join(home, name)
		s, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if s.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink forbidden: %s", p)
		}
	}
	for _, name := range []string{"config/priv_validator_key.json", "config/node_key.json", "keyring-test", "keyring-file", "data/priv_validator_state.json"} {
		if _, e := os.Stat(filepath.Join(home, name)); e == nil {
			return fmt.Errorf("signing material forbidden: %s", name)
		}
	}
	var genesis struct {
		ChainID string `json:"chain_id"`
	}
	read(filepath.Join(home, "config/genesis.json"), &genesis)
	blockTime, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return err
	}
	opts := viper.New()
	opts.Set("home", home)
	opts.Set("inv-check-period", 1)
	opts.Set("pruning", "nothing")
	db, err := dbm.NewDB("application", dbm.GoLevelDBBackend, filepath.Join(home, "data"))
	if err != nil {
		return err
	}
	a := app.NewApp(log.NewNopLogger(), presenceDB{DB: db}, nil, false, opts, baseapp.SetChainID(genesis.ChainID))
	defer a.Close()
	report := map[string]any{"mode": mode, "network_access": false, "signing_keys_used": false, "real_funds_used": false, "chain_id": genesis.ChainID}
	// Wrap the actual registered pre-blocker before BaseApp is sealed. This
	// compares state across migration before inflation/distribution/end-blocks.
	originalPreBlock := a.BaseApp.PreBlocker()
	captured := false
	if mode == "migrate" {
		a.SetPreBlocker(func(ctx sdk.Context, req *abci.RequestFinalizeBlock) (*sdk.ResponsePreBlock, error) {
			plan, e := a.UpgradeKeeper.GetUpgradePlan(ctx)
			if e != nil || plan.Name != planName || plan.Height != req.Height {
				return originalPreBlock(ctx, req)
			}
			before := snapshot(a, ctx)
			result, e := originalPreBlock(ctx, req)
			if e != nil {
				return result, e
			}
			after := snapshot(a, ctx)
			changes := stateDiff(before, after)
			write(filepath.Join(home, "migration-before-stores.json"), digests(before))
			write(filepath.Join(home, "migration-after-stores.json"), digests(after))
			write(filepath.Join(home, "migration-changed-keys.json"), changes)
			for name := range changes {
				if name != "upgrade" && name != "ibc" && name != "transfer" {
					return nil, fmt.Errorf("migration unexpectedly changed %s", name)
				}
			}
			// ibc-go v10 InitGenesis initializes the port and params. SDK
			// module accounts are created lazily when the bank keeper needs
			// them. Therefore even auth's account counter must stay unchanged.
			transferAddress := authtypes.NewModuleAddress("transfer")
			account := a.AccountKeeper.GetAccount(ctx, transferAddress)
			report["transfer_module_account_lazy"] = account == nil
			if len(after["ibc"]) == 0 || len(after["transfer"]) == 0 {
				return nil, fmt.Errorf("IBC stores were not initialized")
			}
			export(a, ctx, filepath.Join(home, "post-migration-genesis.json"))
			report["migration_changed_keys"] = changes
			report["existing_accounts_preserved"] = true
			report["bank_and_business_stores_unchanged_by_migration"] = true
			captured = true
			return result, nil
		})
	}
	if err = a.LoadLatestVersion(); err != nil {
		return err
	}
	height := a.LastBlockHeight()
	hash := fmt.Sprintf("%X", a.LastCommitID().Hash)
	report["loaded_height"] = height
	report["loaded_app_hash"] = hash
	ctx := a.NewUncachedContext(false, cmtproto.Header{ChainID: genesis.ChainID, Height: height, Time: blockTime})
	if mode == "inspect" || mode == "prepare" || mode == "verify" {
		computed := fmt.Sprintf("%X", a.CommitMultiStore().WorkingHash())
		if computed != hash {
			return fmt.Errorf("loaded IAVL roots differ from commit metadata: %s != %s", computed, hash)
		}
		report["recomputed_app_hash"] = computed
		if expected == "" || !strings.EqualFold(hash, expected) {
			return fmt.Errorf("source hash mismatch: loaded %s, expected %s", hash, expected)
		}
		report["checkpoint_app_hash_verified"] = true
		if mode == "verify" {
			var previous struct {
				Applied int64 `json:"applied_upgrade_height"`
				Final   int64 `json:"final_height"`
			}
			read(filepath.Join(home, "migrate-report.json"), &previous)
			done, e := a.UpgradeKeeper.GetDoneHeight(ctx, planName)
			if e != nil {
				return e
			}
			if done != previous.Applied || height != previous.Final {
				return fmt.Errorf("persisted upgrade checkpoint differs")
			}
			vm, e := a.UpgradeKeeper.GetModuleVersionMap(ctx)
			if e != nil {
				return e
			}
			for _, name := range []string{"ibc", "transfer", "07-tendermint"} {
				if _, ok := vm[name]; !ok {
					return fmt.Errorf("missing persisted module %s", name)
				}
			}
			report["applied_upgrade_height"] = done
			report["status"] = "passed"
			write(filepath.Join(home, "verify-report.json"), report)
			return nil
		}
		vm, e := a.UpgradeKeeper.GetModuleVersionMap(ctx)
		if e != nil {
			return e
		}
		report["source_module_versions"] = vm
		write(filepath.Join(home, "source-store-digests.json"), digests(snapshot(a, ctx)))
		export(a, ctx, filepath.Join(home, "source-export.json"))
	}
	if mode == "inspect" {
		report["status"] = "passed"
		write(filepath.Join(home, "inspect-report.json"), report)
		return nil
	}
	var validators struct {
		Validators []struct {
			Address     string `json:"address"`
			VotingPower string `json:"voting_power"`
		} `json:"validators"`
	}
	read(filepath.Join(home, "source-validators.json"), &validators)
	votes := make([]abci.VoteInfo, 0, len(validators.Validators))
	for _, v := range validators.Validators {
		address, e := hex.DecodeString(v.Address)
		if e != nil {
			return e
		}
		var power int64
		if _, e = fmt.Sscan(v.VotingPower, &power); e != nil {
			return e
		}
		votes = append(votes, abci.VoteInfo{Validator: abci.Validator{Address: address, Power: power}, BlockIdFlag: cmtproto.BlockIDFlagCommit})
	}
	if len(votes) == 0 {
		return fmt.Errorf("no production validator fixture")
	}
	block := func(h int64) (*abci.ResponseFinalizeBlock, error) {
		return a.FinalizeBlock(&abci.RequestFinalizeBlock{Height: h, Time: blockTime.Add(time.Duration(h-height) * 30 * time.Second), ProposerAddress: votes[0].Validator.Address, DecidedLastCommit: abci.CommitInfo{Votes: votes}})
	}
	if mode == "prepare" {
		if _, e := a.UpgradeKeeper.GetUpgradePlan(ctx); e == nil {
			return fmt.Errorf("source already has pending upgrade; do not overwrite")
		}
		plan := upgradetypes.Plan{Name: planName, Height: height + 2, Info: "OFFLINE COPY ONLY: plan injected for state migration replay; governance tested separately"}
		if err = a.UpgradeKeeper.ScheduleUpgrade(ctx, plan); err != nil {
			return err
		}
		if _, err = block(height + 1); err != nil {
			return err
		}
		if _, err = a.Commit(); err != nil {
			return err
		}
		ctx = a.NewUncachedContext(false, cmtproto.Header{ChainID: genesis.ChainID, Height: height + 1, Time: blockTime.Add(30 * time.Second)})
		write(filepath.Join(home, "prepared-store-digests.json"), digests(snapshot(a, ctx)))
		report["prepared_height"] = a.LastBlockHeight()
		report["prepared_app_hash"] = fmt.Sprintf("%X", a.LastCommitID().Hash)
		report["upgrade_height"] = plan.Height
		report["plan_injected_offline"] = true
		_, haltErr := block(plan.Height)
		if haltErr == nil || !strings.Contains(haltErr.Error(), "UPGRADE") {
			return fmt.Errorf("expected upgrade halt, got %v", haltErr)
		}
		var info struct {
			Name   string `json:"name"`
			Height int64  `json:"height"`
		}
		read(filepath.Join(home, "data/upgrade-info.json"), &info)
		if info.Name != planName || info.Height != plan.Height {
			return fmt.Errorf("incorrect halt file: %+v", info)
		}
		report["old_binary_halted_at_upgrade"] = true
	} else {
		var prepared map[string]digest
		read(filepath.Join(home, "prepared-store-digests.json"), &prepared)
		loaded := digests(snapshot(a, ctx))
		for name, d := range prepared {
			if loaded[name] != d {
				return fmt.Errorf("store loader changed existing %s", name)
			}
		}
		report["store_loader_preserved_existing_stores"] = true
		for i := int64(1); i <= 3; i++ {
			res, e := block(height + i)
			if e != nil {
				return e
			}
			if len(res.TxResults) != 0 {
				return fmt.Errorf("unexpected transactions")
			}
			if _, e = a.Commit(); e != nil {
				return e
			}
		}
		if !captured {
			return fmt.Errorf("upgrade pre-blocker never executed")
		}
		ctx = a.NewUncachedContext(false, cmtproto.Header{ChainID: genesis.ChainID, Height: a.LastBlockHeight(), Time: blockTime.Add(90 * time.Second)})
		applied, e := a.UpgradeKeeper.GetDoneHeight(ctx, planName)
		if e != nil {
			return e
		}
		if applied != height+1 {
			return fmt.Errorf("incorrect applied height %d", applied)
		}
		vm, e := a.UpgradeKeeper.GetModuleVersionMap(ctx)
		if e != nil {
			return e
		}
		report["post_module_versions"] = vm
		report["applied_upgrade_height"] = applied
		report["resumed_blocks"] = 3
		report["final_height"] = a.LastBlockHeight()
		report["final_app_hash"] = fmt.Sprintf("%X", a.LastCommitID().Hash)
	}
	report["status"] = "passed"
	write(filepath.Join(home, mode+"-report.json"), report)
	fmt.Printf("%s passed: loaded height %d\n", mode, height)
	return nil
}
