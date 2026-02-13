package nodes

import "testing"

func TestKeeperLifecycle(t *testing.T) {
	keeper, err := NewKeeper(DefaultParams())
	if err != nil {
		t.Fatalf("unexpected error constructing keeper: %v", err)
	}

	node := Node{
		Operator: "cosmos1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqew9f9k",
		Stake:    keeper.Params().MinStake,
		Endpoint: "https://worker-1.contentgrid.example",
		Status:   NodeStatusPending,
	}

	if err := keeper.UpsertNode(node); err != nil {
		t.Fatalf("unexpected upsert error: %v", err)
	}

	if err := keeper.UpdateStake(node.Operator, keeper.Params().MinStake+1_000); err != nil {
		t.Fatalf("unexpected stake update error: %v", err)
	}

	if err := keeper.SetStatus(node.Operator, NodeStatusActive); err != nil {
		t.Fatalf("unexpected status update error: %v", err)
	}

	stored, ok := keeper.GetNode(node.Operator)
	if !ok {
		t.Fatalf("expected node to exist after registration")
	}
	if stored.Status != NodeStatusActive {
		t.Fatalf("expected status ACTIVE, got %s", stored.Status)
	}
}
