package app

import (
	"encoding/json"
	"testing"

	"content-grid-chain/x/tokenomics"

	"github.com/cosmos/cosmos-sdk/runtime"
)

func TestDefaultGenesisNotEmpty(t *testing.T) {
	genesis := DefaultGenesis()
	if len(genesis) == 0 {
		t.Fatalf("default genesis should not be empty")
	}
	skip := map[string]struct{}{
		"consensus":        {},
		"params":           {},
		runtime.ModuleName: {},
	}
	for mod, raw := range genesis {
		if len(raw) == 0 {
			if _, ok := skip[mod]; ok {
				continue
			}
			t.Errorf("module %s has empty genesis state", mod)
		}
	}
}

func TestDefaultGenesisUsesTokenomicsDenom(t *testing.T) {
	genesis := DefaultGenesis()

	assertModuleParam := func(module, paramsKey, key string) {
		t.Helper()
		raw := genesis[module]
		if len(raw) == 0 {
			t.Fatalf("missing %s genesis", module)
		}
		var state map[string]any
		if err := json.Unmarshal(raw, &state); err != nil {
			t.Fatalf("unmarshal %s genesis: %v", module, err)
		}
		params, _ := state[paramsKey].(map[string]any)
		if params == nil {
			t.Fatalf("missing %s.%s", module, paramsKey)
		}
		if got, _ := params[key].(string); got != tokenomics.DefaultDenom {
			t.Fatalf("%s.%s.%s = %q, want %q", module, paramsKey, key, got, tokenomics.DefaultDenom)
		}
	}

	assertModuleParam("staking", "params", "bond_denom")
	assertModuleParam("mint", "params", "mint_denom")

	rawGov := genesis["gov"]
	if len(rawGov) == 0 {
		t.Fatalf("missing gov genesis")
	}
	var govState map[string]any
	if err := json.Unmarshal(rawGov, &govState); err != nil {
		t.Fatalf("unmarshal gov genesis: %v", err)
	}
	params, _ := govState["params"].(map[string]any)
	if params == nil {
		t.Fatalf("missing gov.params")
	}
	for _, field := range []string{"min_deposit", "expedited_min_deposit"} {
		coins, _ := params[field].([]any)
		for _, coinAny := range coins {
			coin, _ := coinAny.(map[string]any)
			if coin == nil {
				continue
			}
			if got, _ := coin["denom"].(string); got != tokenomics.DefaultDenom {
				t.Fatalf("gov.params.%s denom = %q, want %q", field, got, tokenomics.DefaultDenom)
			}
		}
	}
}

func TestNewEncodingConfig(t *testing.T) {
	cdc, amino, reg := NewEncodingConfig()
	if cdc == nil {
		t.Fatalf("codec should not be nil")
	}
	if amino == nil {
		t.Fatalf("legacy amino should not be nil")
	}
	if reg == nil {
		t.Fatalf("interface registry should not be nil")
	}
}
