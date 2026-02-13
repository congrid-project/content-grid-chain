package nodes

import "fmt"

// Keeper provides an in-memory representation of registered nodes until
// the runtime wiring is available.
type Keeper struct {
	params Params
	nodes  map[string]Node
}

// NewKeeper constructs a keeper with validated params.
func NewKeeper(params Params) (*Keeper, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	return &Keeper{
		params: params,
		nodes:  make(map[string]Node),
	}, nil
}

// Params returns the active parameters.
func (k *Keeper) Params() Params {
	return k.params
}

// UpsertNode registers or updates a node in-memory after validation.
func (k *Keeper) UpsertNode(node Node) error {
	if err := ValidateNode(node, k.params); err != nil {
		return err
	}
	k.nodes[node.Operator] = node
	return nil
}

// UpdateStake adjusts the stake for an existing operator.
func (k *Keeper) UpdateStake(operator string, stake uint64) error {
	node, ok := k.nodes[operator]
	if !ok {
		return fmt.Errorf("node not found: %s", operator)
	}
	node.Stake = stake
	return k.UpsertNode(node)
}

// SetStatus updates the lifecycle status for an operator.
func (k *Keeper) SetStatus(operator string, status NodeStatus) error {
	node, ok := k.nodes[operator]
	if !ok {
		return fmt.Errorf("node not found: %s", operator)
	}
	node.Status = status
	return k.UpsertNode(node)
}

// GetNode returns a copy of the node if present.
func (k *Keeper) GetNode(operator string) (Node, bool) {
	node, ok := k.nodes[operator]
	return node, ok
}

// ListNodes returns a snapshot of the current nodes.
func (k *Keeper) ListNodes() []Node {
	out := make([]Node, 0, len(k.nodes))
	for _, n := range k.nodes {
		out = append(out, n)
	}
	return out
}
