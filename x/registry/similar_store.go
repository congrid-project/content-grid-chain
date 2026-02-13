package registry

import (
	"encoding/json"
	"fmt"
	"strings"

	"cosmossdk.io/store/prefix"
	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/registry/typespb"
)

var (
	publisherSimilarPrefix = []byte{0x13}
)

type PublisherSimilarStats struct {
	Domain          string `json:"domain"`
	RoundStartUnix  int64  `json:"round_start_unix"`
	VerifiedSimilar int32  `json:"verified_similar_domains"`
	ExpectedSetHash string `json:"expected_set_hash"`
	VerifiedAtUnix  int64  `json:"verified_at_unix"`
}

func (s PublisherSimilarStats) ValidateBasic() error {
	if strings.TrimSpace(s.Domain) == "" {
		return fmt.Errorf("domain required")
	}
	if s.RoundStartUnix <= 0 {
		return fmt.Errorf("round_start_unix must be positive")
	}
	if s.VerifiedSimilar < 0 || s.VerifiedSimilar > SimilarTopN {
		return fmt.Errorf("verified_similar_domains must be within [0,%d]", SimilarTopN)
	}
	if strings.TrimSpace(s.ExpectedSetHash) == "" {
		return fmt.Errorf("expected_set_hash required")
	}
	if s.VerifiedAtUnix <= 0 {
		return fmt.Errorf("verified_at_unix must be positive")
	}
	return nil
}

func (s PublisherSimilarStats) ToProto() *typespb.PublisherSimilarStats {
	return &typespb.PublisherSimilarStats{
		Domain:                 s.Domain,
		RoundStartUnix:         s.RoundStartUnix,
		VerifiedSimilarDomains: s.VerifiedSimilar,
		ExpectedSetHash:        s.ExpectedSetHash,
		VerifiedAtUnix:         s.VerifiedAtUnix,
	}
}

func publisherSimilarKey(domain string) []byte {
	return []byte(NormalizeDomain(domain))
}

func (k Keeper) SetPublisherSimilarStats(ctx sdk.Context, stats PublisherSimilarStats) error {
	stats.Domain = NormalizeDomain(stats.Domain)
	if err := stats.ValidateBasic(); err != nil {
		return err
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), publisherSimilarPrefix)
	bz, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	store.Set(publisherSimilarKey(stats.Domain), bz)
	return nil
}

func (k Keeper) GetPublisherSimilarStats(ctx sdk.Context, domain string) (PublisherSimilarStats, bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), publisherSimilarPrefix)
	bz := store.Get(publisherSimilarKey(domain))
	if len(bz) == 0 {
		return PublisherSimilarStats{}, false
	}
	var stats PublisherSimilarStats
	if err := json.Unmarshal(bz, &stats); err != nil {
		panic(fmt.Errorf("failed to decode similar stats: %w", err))
	}
	return stats, true
}
