package miners

import (
	"encoding/json"
	"fmt"
	"strings"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/miners/typespb"
)

// MinerStatus enumerates lifecycle states.
type MinerStatus int32

const (
	StatusUnspecified MinerStatus = 0
	StatusPending     MinerStatus = 1
	StatusActive      MinerStatus = 2
	StatusJailed      MinerStatus = 3
)

// Service bit flags.
const (
	ServiceFetch      uint32 = 1 << 0
	ServiceEmbed      uint32 = 1 << 1
	ServiceIndex      uint32 = 1 << 2
	ServiceVerify     uint32 = 1 << 3
	ServiceProxy      uint32 = 1 << 4
	ServiceAssignment uint32 = 1 << 5
)

// Miner describes an operator participating in off-chain work.
type Miner struct {
	Operator           string      `json:"operator"`
	MetadataURI        string      `json:"metadata_uri"`
	Services           uint32      `json:"services"`
	MinBid             sdk.Coin    `json:"min_bid"`
	Stake              sdkmath.Int `json:"stake"`
	Status             MinerStatus `json:"status"`
	RegisteredAtHeight int64       `json:"registered_at_height"`
	LastUpdateHeight   int64       `json:"last_update_height"`
}

// Params govern module configuration.
type Params struct {
	StakeDenom        string      `json:"stake_denom"`
	MinStake          sdkmath.Int `json:"min_stake"`
	MaxMetadataLength uint32      `json:"max_metadata_length"`
}

// DefaultParams returns sensible defaults.
func DefaultParams() Params {
	return Params{
		StakeDenom:        "ucongrid",
		MinStake:          sdkmath.NewInt(1_000_000), // 1 CONGRID
		MaxMetadataLength: 256,
	}
}

// Validate ensures params are sane.
func (p Params) Validate() error {
	if strings.TrimSpace(p.StakeDenom) == "" {
		return fmt.Errorf("stake denom required")
	}
	if err := sdk.ValidateDenom(p.StakeDenom); err != nil {
		return fmt.Errorf("invalid stake denom: %w", err)
	}
	if !p.MinStake.IsPositive() {
		return fmt.Errorf("min stake must be positive")
	}
	if p.MaxMetadataLength == 0 {
		return fmt.Errorf("max metadata length must be > 0")
	}
	return nil
}

// ValidateMiner ensures the struct is well-formed.
func ValidateMiner(m Miner, params Params) error {
	if _, err := sdk.AccAddressFromBech32(m.Operator); err != nil {
		return fmt.Errorf("invalid operator: %w", err)
	}
	if strings.TrimSpace(m.MetadataURI) != "" {
		if uint32(len(m.MetadataURI)) > params.MaxMetadataLength {
			return fmt.Errorf("metadata uri exceeds max length %d", params.MaxMetadataLength)
		}
	}
	if m.Services == 0 {
		return fmt.Errorf("miner must advertise at least one service")
	}
	if !m.MinBid.IsValid() || !m.MinBid.IsPositive() {
		return fmt.Errorf("min bid must be a positive coin")
	}
	if m.MinBid.Denom != params.StakeDenom {
		return fmt.Errorf("min bid denom must be %s", params.StakeDenom)
	}
	if !m.Stake.IsPositive() {
		return fmt.Errorf("stake must be positive")
	}
	if m.Stake.LT(params.MinStake) {
		return fmt.Errorf("stake must be >= %s", params.MinStake.String())
	}
	if m.Status < StatusPending || m.Status > StatusJailed {
		return fmt.Errorf("invalid status: %d", m.Status)
	}
	return nil
}

// ToProto converts Miner to protobuf.
func (m Miner) ToProto() *typespb.Miner {
	return &typespb.Miner{
		Operator:           m.Operator,
		MetadataUri:        m.MetadataURI,
		Services:           m.Services,
		MinBidDenom:        m.MinBid.Denom,
		MinBidAmount:       m.MinBid.Amount.String(),
		Stake:              m.Stake.String(),
		Status:             typespb.MinerStatus(m.Status),
		RegisteredAtHeight: m.RegisteredAtHeight,
		LastUpdateHeight:   m.LastUpdateHeight,
	}
}

// MinerFromProto converts protobuf representation.
func MinerFromProto(pb *typespb.Miner) (Miner, error) {
	if pb == nil {
		return Miner{}, fmt.Errorf("nil miner proto")
	}
	minBid, ok := sdkmath.NewIntFromString(pb.GetMinBidAmount())
	if !ok {
		return Miner{}, fmt.Errorf("invalid min bid amount")
	}
	stake, ok := sdkmath.NewIntFromString(pb.GetStake())
	if !ok {
		return Miner{}, fmt.Errorf("invalid stake amount")
	}
	return Miner{
		Operator:           pb.GetOperator(),
		MetadataURI:        pb.GetMetadataUri(),
		Services:           pb.GetServices(),
		MinBid:             sdk.NewCoin(pb.GetMinBidDenom(), minBid),
		Stake:              stake,
		Status:             MinerStatus(pb.GetStatus()),
		RegisteredAtHeight: pb.GetRegisteredAtHeight(),
		LastUpdateHeight:   pb.GetLastUpdateHeight(),
	}, nil
}

func marshalMiner(m Miner) ([]byte, error) {
	return json.Marshal(m)
}

func unmarshalMiner(bz []byte) (Miner, error) {
	var m Miner
	err := json.Unmarshal(bz, &m)
	return m, err
}
