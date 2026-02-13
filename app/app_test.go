package app

import (
	"testing"

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
