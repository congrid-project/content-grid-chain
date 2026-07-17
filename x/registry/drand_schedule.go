package registry

import (
	"fmt"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// DrandRequirement describes the single drand beacon accepted for the next
// verification round that has not yet been assigned.
type DrandRequirement struct {
	Enabled            bool
	Pending            bool
	RoundStartUnix     int64
	RequiredDrandRound uint64
	RequiredBeaconUnix int64
	Submitted          bool
	DrandChainHash     string
}

func nextVerificationRoundStartUnix(now time.Time, intervalSeconds int64) int64 {
	if intervalSeconds <= 0 {
		intervalSeconds = int64(time.Hour.Seconds())
	}
	interval := time.Duration(intervalSeconds) * time.Second
	return now.UTC().Truncate(interval).Add(interval).Unix()
}

// requiredDrandRound deterministically maps a Content Grid verification round
// to one drand round. Drand round 1 is emitted at genesis_time; subsequent
// rounds are period_seconds apart.
func requiredDrandRound(params PublisherParams, roundStartUnix int64) (uint64, int64, error) {
	genesisUnix := params.EffectiveDrandGenesisTimeUnix()
	periodSeconds := params.EffectiveDrandPeriodSeconds()
	offsetSeconds := params.EffectiveDrandRoundOffsetSeconds()
	if roundStartUnix <= 0 {
		return 0, 0, fmt.Errorf("round start unix must be positive")
	}
	if genesisUnix <= 0 || periodSeconds <= 0 || offsetSeconds <= 0 {
		return 0, 0, fmt.Errorf("invalid drand schedule parameters")
	}

	latestAllowedUnix := roundStartUnix - offsetSeconds
	if latestAllowedUnix < genesisUnix {
		return 0, 0, fmt.Errorf("verification round predates drand genesis")
	}
	round := uint64((latestAllowedUnix-genesisUnix)/periodSeconds) + 1
	beaconUnix := genesisUnix + (int64(round)-1)*periodSeconds
	return round, beaconUnix, nil
}

// PendingDrandRequirement returns the exact beacon accepted by
// MsgSubmitDrandBeacon at the current block time.
func (k Keeper) PendingDrandRequirement(ctx sdk.Context) (DrandRequirement, error) {
	params := k.GetParams(ctx)
	out := DrandRequirement{
		Enabled:        params.EffectiveDrandEnabled(),
		DrandChainHash: params.EffectiveDrandChainHash(),
	}
	if !out.Enabled {
		return out, nil
	}

	roundStartUnix := nextVerificationRoundStartUnix(ctx.BlockTime(), params.RoundIntervalSeconds)
	out.RoundStartUnix = roundStartUnix
	if last, ok := k.GetLastRoundStart(ctx); ok && roundStartUnix <= last {
		return out, nil
	}

	requiredRound, beaconUnix, err := requiredDrandRound(params, roundStartUnix)
	if err != nil {
		return DrandRequirement{}, err
	}
	out.Pending = true
	out.RequiredDrandRound = requiredRound
	out.RequiredBeaconUnix = beaconUnix
	_, out.Submitted = k.GetDrandBeacon(ctx, requiredRound)
	return out, nil
}
