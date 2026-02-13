package nodes

import "testing"

func TestValidateNode(t *testing.T) {
	params := DefaultParams()
	node := Node{
		Operator: "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqew9f9k",
		Stake:    params.MinStake,
		Endpoint: "https://worker-1.contentgrid.example:26657",
		Status:   NodeStatusActive,
	}
	if err := ValidateNode(node, params); err != nil {
		t.Fatalf("unexpected error validating node: %v", err)
	}

	node.Stake = params.MinStake - 1
	if err := ValidateNode(node, params); err == nil {
		t.Fatalf("expected insufficient stake error")
	}
}

func TestGenesisValidate(t *testing.T) {
	gs := DefaultGenesis()
	if err := gs.Validate(); err != nil {
		t.Fatalf("default genesis should validate: %v", err)
	}

	bad := GenesisState{
		Params: DefaultParams(),
		Nodes: []Node{
			{
				Operator: "",
				Stake:    DefaultParams().MinStake,
				Status:   NodeStatusPending,
			},
		},
	}
	if err := bad.Validate(); err == nil {
		t.Fatalf("expected validation error for empty operator")
	}
}
