package nodes

import (
	"errors"
	"fmt"
	"net/url"
)

// NodeStatus represents the current lifecycle status of a network node.
type NodeStatus int32

const (
	NodeStatusPending NodeStatus = 0
	NodeStatusActive  NodeStatus = 1
	NodeStatusJailed  NodeStatus = 2
)

func (s NodeStatus) String() string {
	switch s {
	case NodeStatusPending:
		return "PENDING"
	case NodeStatusActive:
		return "ACTIVE"
	case NodeStatusJailed:
		return "JAILED"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(s))
	}
}

// Node describes a staking operator registered with the network.
type Node struct {
	Operator string     `json:"operator"`
	Stake    uint64     `json:"stake"`
	Endpoint string     `json:"endpoint"`
	Status   NodeStatus `json:"status"`
}

// Params define module-level configuration for node registration.
type Params struct {
	MinStake          uint64 `json:"min_stake"`
	MaxEndpointLength int    `json:"max_endpoint_length"`
}

// DefaultParams returns conservative defaults suitable for tests.
func DefaultParams() Params {
	return Params{
		MinStake:          1_000_000, // 1 token if token has 6 decimals
		MaxEndpointLength: 256,
	}
}

// Validate ensures the params are sane.
func (p Params) Validate() error {
	if p.MinStake == 0 {
		return errors.New("min stake must be positive")
	}
	if p.MaxEndpointLength <= 0 {
		return errors.New("max endpoint length must be positive")
	}
	return nil
}

// ValidateNode performs semantic checks on a node record.
func ValidateNode(n Node, params Params) error {
	if n.Operator == "" {
		return errors.New("operator address required")
	}
	if n.Stake < params.MinStake {
		return fmt.Errorf("insufficient stake: have %d need >= %d", n.Stake, params.MinStake)
	}
	if n.Status < NodeStatusPending || n.Status > NodeStatusJailed {
		return fmt.Errorf("invalid status: %d", n.Status)
	}
	if len(n.Endpoint) > params.MaxEndpointLength {
		return fmt.Errorf("endpoint too long: %d > %d", len(n.Endpoint), params.MaxEndpointLength)
	}
	if n.Endpoint != "" {
		if _, err := url.ParseRequestURI(n.Endpoint); err != nil {
			return fmt.Errorf("invalid endpoint url: %w", err)
		}
	}
	return nil
}

// GenesisState defines the module state at chain start.
type GenesisState struct {
	Params Params `json:"params"`
	Nodes  []Node `json:"nodes"`
}

// DefaultGenesis returns an empty but valid genesis state.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params: DefaultParams(),
		Nodes:  []Node{},
	}
}

// Validate ensures the genesis configuration is self-consistent.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	seen := make(map[string]struct{})
	for _, n := range gs.Nodes {
		if err := ValidateNode(n, gs.Params); err != nil {
			return err
		}
		if _, ok := seen[n.Operator]; ok {
			return fmt.Errorf("duplicate operator in genesis: %s", n.Operator)
		}
		seen[n.Operator] = struct{}{}
	}

	return nil
}
