package registry

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	registrypb "content-grid-chain/x/registry/typespb"
	"content-grid-chain/x/tokenomics"
	"content-grid-chain/x/verifiers"
)

const verificationRewardDenom = tokenomics.DefaultDenom

// EndBlock processes verification finalization and assignment creation.
func (k Keeper) EndBlock(ctx sdk.Context) error {
	if err := k.finalizeAssignments(ctx); err != nil {
		return err
	}
	if err := k.settleLeases(ctx); err != nil {
		return err
	}
	if err := k.assignNewRound(ctx); err != nil {
		return err
	}
	return nil
}

func (k Keeper) assignNewRound(ctx sdk.Context) error {
	params := k.GetParams(ctx)
	if params.SubmissionWindowSeconds <= 0 {
		return fmt.Errorf("submission window seconds must be positive")
	}
	intervalSeconds := params.RoundIntervalSeconds
	if intervalSeconds <= 0 {
		intervalSeconds = int64(time.Hour.Seconds())
	}
	assignmentDelayMaxSeconds := params.AssignmentDelayMaxSeconds
	if assignmentDelayMaxSeconds <= 0 || assignmentDelayMaxSeconds > intervalSeconds {
		assignmentDelayMaxSeconds = intervalSeconds
	}
	// Always schedule newly-created assignments for the NEXT round boundary,
	// so publisher registration never triggers immediate same-round verification.
	roundStart := ctx.BlockTime().UTC().Truncate(time.Duration(intervalSeconds) * time.Second).Add(time.Duration(intervalSeconds) * time.Second)
	roundStartUnix := roundStart.Unix()
	if roundStartUnix <= 0 {
		return nil
	}
	if last, ok := k.GetLastRoundStart(ctx); ok && roundStartUnix <= last {
		return nil
	}

	publishers := make([]Website, 0)
	nowUnix := roundStartUnix
	k.IterateWebsites(ctx, func(w Website) bool {
		if w.CooldownUntilUnix > nowUnix {
			return false
		}
		if w.Status == StatusPending {
			publishers = append(publishers, w)
			return false
		}
		if w.Status == StatusVerified {
			if len(k.activeLeasesForDomain(ctx, w.Domain, nowUnix)) > 0 {
				publishers = append(publishers, w)
			}
		}
		return false
	})
	sort.Slice(publishers, func(i, j int) bool { return publishers[i].Domain < publishers[j].Domain })

	eligible, verifierStakeByAddr, err := k.eligibleVerifierAddrs(ctx)
	if err != nil {
		return err
	}

	anchorHeight := ctx.BlockHeight() - 1
	if anchorHeight < 0 {
		anchorHeight = 0
	}
	anchorHash := append([]byte(nil), ctx.BlockHeader().LastBlockId.Hash...)
	roundSeed := registrypb.ComputeRoundSeedWithAnchor(ctx.ChainID(), roundStartUnix, anchorHeight, anchorHash)

	var usedDrandRound uint64
	var usedDrandRandomnessHex string
	if params.EffectiveDrandEnabled() {
		beacon, found := k.GetLatestDrandBeacon(ctx)
		if !found {
			if params.EffectiveDrandStrictMode() {
				ctx.EventManager().EmitEvent(
					sdk.NewEvent(
						EventTypeDrandBeaconSkipped,
						sdk.NewAttribute(AttributeKeyRoundStart, fmt.Sprintf("%d", roundStartUnix)),
						sdk.NewAttribute(AttributeKeyStatus, "missing_latest_beacon"),
					),
				)
				return nil
			}
		} else {
			randomness, err := hex.DecodeString(strings.TrimSpace(beacon.RandomnessHex))
			if err != nil {
				if params.EffectiveDrandStrictMode() {
					ctx.EventManager().EmitEvent(
						sdk.NewEvent(
							EventTypeDrandBeaconSkipped,
							sdk.NewAttribute(AttributeKeyRoundStart, fmt.Sprintf("%d", roundStartUnix)),
							sdk.NewAttribute(AttributeKeyStatus, "invalid_latest_beacon"),
						),
					)
					return nil
				}
			} else {
				roundSeed = registrypb.ComputeRoundSeedWithDrand(ctx.ChainID(), roundStartUnix, anchorHeight, anchorHash, beacon.Round, randomness)
				usedDrandRound = beacon.Round
				usedDrandRandomnessHex = strings.TrimSpace(beacon.RandomnessHex)
			}
		}
	}

	if _, found := k.GetRoundMeta(ctx, roundStartUnix); !found {
		verifierSetHash := hashVerifierSet(eligible)
		if err := k.SetRoundMeta(ctx, VerificationRoundMeta{
			RoundStartUnix:            roundStartUnix,
			SeedHex:                   fmt.Sprintf("%x", roundSeed[:]),
			RoundIntervalSeconds:      intervalSeconds,
			AssignmentDelayMaxSeconds: assignmentDelayMaxSeconds,
			CreatedAtUnix:             ctx.BlockTime().UTC().Unix(),
			AnchorHeight:              anchorHeight,
			AnchorHashHex:             hex.EncodeToString(anchorHash),
			VerifierSetHash:           fmt.Sprintf("%x", verifierSetHash[:]),
			VerifierSetSize:           int32(len(eligible)),
			DrandRound:                usedDrandRound,
			DrandRandomnessHex:        usedDrandRandomnessHex,
		}); err != nil {
			return err
		}
	}

	for _, w := range publishers {
		if _, found := k.GetAssignment(ctx, roundStartUnix, w.Domain); found {
			continue
		}
		startAtUnix := registrypb.ComputeAssignmentStartAtUnix(roundSeed, w.Domain, roundStartUnix, intervalSeconds, assignmentDelayMaxSeconds)
		deadlineUnix := startAtUnix + params.SubmissionWindowSeconds

		selected := selectDeterministicWeighted(
			eligible,
			verifierStakeByAddr,
			params.MinVerifierCount,
			append(append([]byte{}, roundSeed[:]...), []byte(w.Domain)...),
		)
		sort.Strings(selected)

		assignment := PublisherVerificationAssignment{
			RoundStartUnix:  roundStartUnix,
			Domain:          w.Domain,
			StartAtUnix:     startAtUnix,
			DeadlineUnix:    deadlineUnix,
			Verifiers:       selected,
			Finalized:       false,
			FinalizedAtUnix: 0,
		}
		if err := k.SetAssignment(ctx, assignment); err != nil {
			return err
		}
		ctx.EventManager().EmitEvent(
			sdk.NewEvent(
				EventTypeVerificationAssigned,
				sdk.NewAttribute(AttributeKeyDomain, assignment.Domain),
				sdk.NewAttribute(AttributeKeyRoundStart, fmt.Sprintf("%d", assignment.RoundStartUnix)),
				sdk.NewAttribute(AttributeKeyStartAt, fmt.Sprintf("%d", assignment.StartAtUnix)),
				sdk.NewAttribute(AttributeKeyDeadline, fmt.Sprintf("%d", assignment.DeadlineUnix)),
				sdk.NewAttribute(AttributeKeyVerifierCount, fmt.Sprintf("%d", len(assignment.Verifiers))),
			),
		)
	}

	k.SetLastRoundStart(ctx, roundStartUnix)
	return nil
}

func (k Keeper) finalizeAssignments(ctx sdk.Context) error {
	nowUnix := ctx.BlockTime().UTC().Unix()
	params := k.GetParams(ctx)
	var errOut error

	k.IterateAssignments(ctx, func(a PublisherVerificationAssignment) bool {
		if a.Finalized || a.DeadlineUnix > nowUnix {
			return false
		}

		submissions := k.ListSubmissions(ctx, a.RoundStartUnix, a.Domain)
		submissionByVerifier := make(map[string]PublisherVerificationSubmission, len(submissions))
		for _, sub := range submissions {
			submissionByVerifier[sub.Verifier] = sub
		}

		stats := computeSimilarRoundStats(len(a.Verifiers), submissions)
		passes, fails := stats.Passes, stats.Fails

		quorum := stats.Quorum
		hasQuorum := len(submissions) >= quorum && quorum > 0
		majorityPass := hasQuorum && passes > fails
		verified := hasQuorum && majorityPass

		intervalSeconds := params.RoundIntervalSeconds
		if intervalSeconds <= 0 {
			intervalSeconds = int64(time.Hour.Seconds())
		}

		if website, found := k.GetWebsite(ctx, a.Domain); found {
			if verified {
				if website.Status != StatusVerified {
					website.Status = StatusVerified
					if err := k.UpsertWebsite(ctx, website); err != nil {
						errOut = err
						return true
					}
				}
				if err := k.rewardVerifiedPublisher(ctx, a, website, submissions, stats.VerifiedSimilarDomains, intervalSeconds, params); err != nil {
					errOut = err
					return true
				}
				k.ClearPublisherFailureStreak(ctx, website.Domain)

				// Persist latest similar-site settlement for easy querying.
				if stats.MajorityExpectedHash != "" {
					_ = k.SetPublisherSimilarStats(ctx, PublisherSimilarStats{
						Domain:          website.Domain,
						RoundStartUnix:  a.RoundStartUnix,
						VerifiedSimilar: stats.VerifiedSimilarDomains,
						ExpectedSetHash: stats.MajorityExpectedHash,
						VerifiedAtUnix:  nowUnix,
					})
				}

				if updated, cleared, err := k.clearPublisherCooldown(ctx, website); err != nil {
					errOut = err
					return true
				} else if cleared {
					website = updated
				}

				ctx.EventManager().EmitEvent(
					sdk.NewEvent(
						EventTypePublisherVerified,
						sdk.NewAttribute(AttributeKeyDomain, website.Domain),
						sdk.NewAttribute(AttributeKeyOwner, website.Owner),
						sdk.NewAttribute(AttributeKeyRoundStart, fmt.Sprintf("%d", a.RoundStartUnix)),
						sdk.NewAttribute(AttributeKeyExpectedHash, stats.MajorityExpectedHash),
						sdk.NewAttribute(AttributeKeyMatchedSimilar, fmt.Sprintf("%d", stats.VerifiedSimilarDomains)),
					),
				)
			} else if hasQuorum {
				if website.Status == StatusVerified || website.Status == StatusPending {
					failCount := k.IncrementPublisherFailureStreak(ctx, website.Domain)
					if website.Status == StatusVerified {
						revokeThreshold := params.EffectivePublisherRevokeFailureThreshold()
						if failCount >= revokeThreshold {
							website.Status = StatusRevoked
						} else {
							website.Status = StatusPending
						}
						if err := k.UpsertWebsite(ctx, website); err != nil {
							errOut = err
							return true
						}
					}
				}
				if len(k.activeLeasesForDomain(ctx, website.Domain, nowUnix)) > 0 {
					if website.CooldownUntilUnix <= nowUnix {
						updated, _, err := k.applyPublisherCooldown(ctx, website, params, nowUnix)
						if err != nil {
							errOut = err
							return true
						}
						website = updated
						if err := k.violateActiveLeasesForDomain(ctx, website.Domain, nowUnix); err != nil {
							errOut = err
							return true
						}
						ctx.EventManager().EmitEvent(
							sdk.NewEvent(
								EventTypePublisherCooldown,
								sdk.NewAttribute(AttributeKeyDomain, website.Domain),
								sdk.NewAttribute(AttributeKeyOwner, website.Owner),
								sdk.NewAttribute(AttributeKeyRoundStart, fmt.Sprintf("%d", a.RoundStartUnix)),
								sdk.NewAttribute(AttributeKeyCooldownUntil, fmt.Sprintf("%d", website.CooldownUntilUnix)),
								sdk.NewAttribute(AttributeKeyCooldownCount, fmt.Sprintf("%d", website.CooldownCount)),
							),
						)
					}
				}
			}
		}

		for _, verifierAddr := range a.Verifiers {
			sub, submitted := submissionByVerifier[verifierAddr]
			penaltyReason := ""
			penalize := false

			if !submitted {
				penalize = true
				if hasQuorum {
					penaltyReason = "miss"
				} else {
					penaltyReason = "miss_no_quorum"
				}
			} else if hasQuorum && sub.Passed != majorityPass {
				penalize = true
				penaltyReason = "wrong_majority"
			}

			if penalize {
				state := k.ApplyVerifierPenalty(
					ctx,
					verifierAddr,
					nowUnix,
					intervalSeconds,
					params.EffectiveVerifierPenaltySuspendThreshold(),
					params.EffectiveVerifierPenaltySuspendRounds(),
				)
				ctx.EventManager().EmitEvent(
					sdk.NewEvent(
						EventTypeVerifierPenalized,
						sdk.NewAttribute(AttributeKeyVerifier, verifierAddr),
						sdk.NewAttribute(AttributeKeyDomain, a.Domain),
						sdk.NewAttribute(AttributeKeyRoundStart, fmt.Sprintf("%d", a.RoundStartUnix)),
						sdk.NewAttribute(AttributeKeyPenaltyReason, penaltyReason),
						sdk.NewAttribute(AttributeKeyPenaltyCount, fmt.Sprintf("%d", state.Count)),
						sdk.NewAttribute(AttributeKeySuspendedUntil, fmt.Sprintf("%d", state.SuspendedUntilUnix)),
					),
				)
				continue
			}
			k.ClearVerifierPenalty(ctx, verifierAddr)
		}

		a.Finalized = true
		a.FinalizedAtUnix = nowUnix
		if err := k.SetAssignment(ctx, a); err != nil {
			errOut = err
			return true
		}

		ctx.EventManager().EmitEvent(
			sdk.NewEvent(
				EventTypeVerificationFinalized,
				sdk.NewAttribute(AttributeKeyDomain, a.Domain),
				sdk.NewAttribute(AttributeKeyRoundStart, fmt.Sprintf("%d", a.RoundStartUnix)),
				sdk.NewAttribute(AttributeKeyPasses, fmt.Sprintf("%d", passes)),
				sdk.NewAttribute(AttributeKeyFails, fmt.Sprintf("%d", fails)),
				sdk.NewAttribute(AttributeKeyQuorum, fmt.Sprintf("%d", quorum)),
				sdk.NewAttribute(AttributeKeySubmissionCount, fmt.Sprintf("%d", len(submissions))),
				sdk.NewAttribute(AttributeKeyVerified, fmt.Sprintf("%t", hasQuorum && majorityPass)),
				sdk.NewAttribute(AttributeKeyExpectedHash, stats.MajorityExpectedHash),
				sdk.NewAttribute(AttributeKeyMatchedSimilar, fmt.Sprintf("%d", stats.VerifiedSimilarDomains)),
			),
		)
		return false
	})

	return errOut
}

func (k Keeper) eligibleVerifierAddrs(ctx sdk.Context) ([]string, map[string]sdkmath.Int, error) {
	if k.verifiers == nil {
		return nil, nil, nil
	}
	vparams := k.verifiers.GetParams(ctx)
	all := k.verifiers.ListVerifiers(ctx)
	eligible := make([]string, 0, len(all))
	stakeByAddr := make(map[string]sdkmath.Int, len(all))
	nowUnix := ctx.BlockTime().UTC().Unix()
	for _, v := range all {
		if v.Status != verifiers.StatusActive {
			continue
		}
		if v.Bond.Denom != vparams.BondDenom {
			continue
		}
		if v.Bond.Amount.LT(vparams.MinBond) {
			continue
		}
		if k.IsVerifierSuspended(ctx, v.Address, nowUnix) {
			continue
		}
		eligible = append(eligible, v.Address)
		stakeByAddr[v.Address] = v.Bond.Amount
	}
	sort.Strings(eligible)
	return eligible, stakeByAddr, nil
}

func requiredQuorum(assigned int) int {
	if assigned <= 0 {
		return 0
	}
	return (assigned + 1) / 2
}

func hashVerifierSet(verifiers []string) [32]byte {
	list := append([]string(nil), verifiers...)
	sort.Strings(list)
	h := sha256.New()
	for _, addr := range list {
		norm := strings.TrimSpace(addr)
		_, _ = h.Write([]byte(norm))
		_, _ = h.Write([]byte{0})
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

func selectDeterministicWeighted(candidates []string, stakeByAddr map[string]sdkmath.Int, k int, seed []byte) []string {
	if k <= 0 || len(candidates) == 0 {
		return nil
	}

	// Fall back to deterministic full set when requested count covers all candidates.
	if len(candidates) <= k {
		out := append([]string(nil), candidates...)
		sort.Strings(out)
		return out
	}

	type weightedCandidate struct {
		addr   string
		weight sdkmath.Int
	}
	pool := make([]weightedCandidate, 0, len(candidates))
	for _, addr := range candidates {
		weight := stakeByAddr[addr]
		if !weight.IsPositive() {
			// Defensive fallback for any unexpected zero/missing stake.
			weight = sdkmath.OneInt()
		}
		pool = append(pool, weightedCandidate{addr: addr, weight: weight})
	}

	selected := make([]string, 0, k)
	for draw := 0; draw < k && len(pool) > 0; draw++ {
		totalWeight := big.NewInt(0)
		for _, entry := range pool {
			totalWeight.Add(totalWeight, entry.weight.BigInt())
		}
		if totalWeight.Sign() <= 0 {
			break
		}

		pick := deterministicPick(seed, draw, totalWeight)
		running := big.NewInt(0)
		pickedIdx := len(pool) - 1
		for i, entry := range pool {
			running.Add(running, entry.weight.BigInt())
			if running.Cmp(pick) == 1 {
				pickedIdx = i
				break
			}
		}

		selected = append(selected, pool[pickedIdx].addr)
		pool = append(pool[:pickedIdx], pool[pickedIdx+1:]...)
	}

	if len(selected) < k {
		remaining := make([]string, 0, len(pool))
		for _, entry := range pool {
			remaining = append(remaining, entry.addr)
		}
		sort.Strings(remaining)
		for _, addr := range remaining {
			if len(selected) >= k {
				break
			}
			selected = append(selected, addr)
		}
	}
	return selected
}

func deterministicPick(seed []byte, draw int, totalWeight *big.Int) *big.Int {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(draw))

	h := sha256.New()
	_, _ = h.Write(seed)
	_, _ = h.Write(buf)
	hashed := h.Sum(nil)

	pick := new(big.Int).SetBytes(hashed)
	pick.Mod(pick, totalWeight)
	return pick
}

func splitVerifierAssignmentRewards(total sdkmath.Int, successful []string, weightByAddr map[string]sdkmath.Int, baseShareBps int64) (map[string]sdkmath.Int, sdkmath.Int) {
	payoutByAddr := make(map[string]sdkmath.Int, len(successful))
	if !total.IsPositive() || len(successful) == 0 {
		return payoutByAddr, total
	}
	if baseShareBps < 0 {
		baseShareBps = 0
	}
	if baseShareBps > 10000 {
		baseShareBps = 10000
	}

	remaining := total
	basePool := total.MulRaw(baseShareBps).QuoRaw(10000)
	weightedPool := total.Sub(basePool)

	if basePool.IsPositive() {
		per := basePool.QuoRaw(int64(len(successful)))
		distributed := sdkmath.ZeroInt()
		for i, addr := range successful {
			share := per
			if i == len(successful)-1 {
				share = basePool.Sub(distributed)
			}
			if !share.IsPositive() {
				continue
			}
			if existing, ok := payoutByAddr[addr]; ok {
				payoutByAddr[addr] = existing.Add(share)
			} else {
				payoutByAddr[addr] = share
			}
			distributed = distributed.Add(share)
			remaining = remaining.Sub(share)
		}
	}

	type weightedVerifier struct {
		addr   string
		weight sdkmath.Int
	}
	weighted := make([]weightedVerifier, 0, len(successful))
	totalWeight := sdkmath.ZeroInt()
	for _, addr := range successful {
		weight, ok := weightByAddr[addr]
		if !ok || !weight.IsPositive() {
			continue
		}
		weighted = append(weighted, weightedVerifier{addr: addr, weight: weight})
		totalWeight = totalWeight.Add(weight)
	}

	if weightedPool.IsPositive() && len(weighted) > 0 && totalWeight.IsPositive() {
		distributed := sdkmath.ZeroInt()
		for i, entry := range weighted {
			share := sdkmath.ZeroInt()
			if i == len(weighted)-1 {
				share = weightedPool.Sub(distributed)
			} else {
				share = weightedPool.Mul(entry.weight).Quo(totalWeight)
			}
			if !share.IsPositive() {
				continue
			}
			if existing, ok := payoutByAddr[entry.addr]; ok {
				payoutByAddr[entry.addr] = existing.Add(share)
			} else {
				payoutByAddr[entry.addr] = share
			}
			distributed = distributed.Add(share)
			remaining = remaining.Sub(share)
		}
	}

	if remaining.IsNegative() {
		remaining = sdkmath.ZeroInt()
	}
	return payoutByAddr, remaining
}

func (k Keeper) rewardVerifiedPublisher(ctx sdk.Context, assignment PublisherVerificationAssignment, website Website, submissions []PublisherVerificationSubmission, matchedExternalLinks int32, intervalSeconds int64, params PublisherParams) error {
	if k.tokenomics == nil {
		return nil
	}

	roundPublisherCount := k.CountAssignmentsForRound(ctx, assignment.RoundStartUnix)
	if roundPublisherCount <= 0 {
		roundPublisherCount = 1
	}

	emissionPoolBps := params.EffectivePublisherEmissionBps() + params.EffectiveVerifierEmissionBps()
	if emissionPoolBps > 0 {
		totalPoolAmount := params.EffectiveEmissionTotalSupply().MulRaw(emissionPoolBps).QuoRaw(10000)
		if totalPoolAmount.IsPositive() {
			if err := k.tokenomics.EnsureEmissionPool(ctx, verificationRewardDenom, totalPoolAmount); err != nil {
				return err
			}
		}
	}

	publisherRoundPool, verifierRoundPool, err := params.RoundEmissionPools(intervalSeconds)
	if err != nil {
		return err
	}
	if publisherRoundPool.IsZero() && params.PublisherVerificationReward.IsPositive() {
		publisherRoundPool = params.PublisherVerificationReward
	}
	if verifierRoundPool.IsZero() && params.VerifierVerificationReward.IsPositive() {
		verifierRoundPool = params.VerifierVerificationReward
	}

	if publisherRoundPool.IsPositive() {
		publisherPoolPerAssignment := publisherRoundPool.QuoRaw(int64(roundPublisherCount))
		if publisherPoolPerAssignment.IsPositive() {
			requiredLinks := params.EffectiveRequiredExternalLinksForFullReward()
			claimable := publisherPoolPerAssignment
			if requiredLinks > 0 {
				if matchedExternalLinks <= 0 {
					claimable = sdkmath.ZeroInt()
				} else if matchedExternalLinks < requiredLinks {
					claimable = publisherPoolPerAssignment.MulRaw(int64(matchedExternalLinks)).QuoRaw(int64(requiredLinks))
				}
			}
			if claimable.GT(publisherPoolPerAssignment) {
				claimable = publisherPoolPerAssignment
			}
			unclaimed := publisherPoolPerAssignment.Sub(claimable)

			if claimable.IsPositive() {
				ownerAddr, err := sdk.AccAddressFromBech32(website.Owner)
				if err != nil {
					return err
				}
				coin := sdk.NewCoin(verificationRewardDenom, claimable)
				if err := k.tokenomics.SendFromPool(ctx, ownerAddr, sdk.NewCoins(coin)); err != nil {
					return err
				}
			}
			if unclaimed.IsPositive() {
				if err := k.tokenomics.BurnFromPool(ctx, sdk.NewCoins(sdk.NewCoin(verificationRewardDenom, unclaimed))); err != nil {
					return err
				}
			}
		}
	}

	if !verifierRoundPool.IsPositive() {
		return nil
	}

	verifierPoolPerAssignment := verifierRoundPool.QuoRaw(int64(roundPublisherCount))
	if !verifierPoolPerAssignment.IsPositive() {
		return nil
	}

	successfulSet := make(map[string]struct{}, len(submissions))
	for _, sub := range submissions {
		if sub.Passed {
			successfulSet[sub.Verifier] = struct{}{}
		}
	}
	if len(successfulSet) == 0 {
		return k.tokenomics.BurnFromPool(ctx, sdk.NewCoins(sdk.NewCoin(verificationRewardDenom, verifierPoolPerAssignment)))
	}

	successful := make([]string, 0, len(successfulSet))
	for addr := range successfulSet {
		successful = append(successful, addr)
	}
	sort.Strings(successful)

	bondByAddr := map[string]sdkmath.Int{}
	bondDenom := verificationRewardDenom
	if k.verifiers != nil {
		bondDenom = k.verifiers.GetParams(ctx).BondDenom
		for _, v := range k.verifiers.ListVerifiers(ctx) {
			if v.Bond.Denom != bondDenom {
				continue
			}
			bondByAddr[v.Address] = v.Bond.Amount
		}
	}

	weightByAddr := make(map[string]sdkmath.Int, len(successful))
	for _, addr := range successful {
		stake, ok := bondByAddr[addr]
		if !ok || !stake.IsPositive() {
			continue
		}
		refActive := k.CountActiveReferredPublishers(ctx, addr)
		refFactor := int64(refActive)
		if refFactor <= 0 {
			refFactor = 1
		}
		weight := stake.MulRaw(refFactor)
		if weight.IsPositive() {
			weightByAddr[addr] = weight
		}
	}

	payoutByAddr, remaining := splitVerifierAssignmentRewards(
		verifierPoolPerAssignment,
		successful,
		weightByAddr,
		params.EffectiveVerifierRewardBaseShareBps(),
	)

	for _, addr := range successful {
		share := payoutByAddr[addr]
		if !share.IsPositive() {
			continue
		}
		acc, err := sdk.AccAddressFromBech32(addr)
		if err != nil {
			return err
		}
		coin := sdk.NewCoin(verificationRewardDenom, share)
		if err := k.tokenomics.SendFromPool(ctx, acc, sdk.NewCoins(coin)); err != nil {
			return err
		}
	}
	if remaining.IsPositive() {
		if err := k.tokenomics.BurnFromPool(ctx, sdk.NewCoins(sdk.NewCoin(verificationRewardDenom, remaining))); err != nil {
			return err
		}
	}
	return nil
}
