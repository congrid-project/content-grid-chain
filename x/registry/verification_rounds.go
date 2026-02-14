package registry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

	eligible, err := k.eligibleVerifierAddrs(ctx)
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

		selected := selectDeterministic(eligible, params.MinVerifierCount, append(append([]byte{}, roundSeed[:]...), []byte(w.Domain)...))
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
				if website.Status == StatusVerified {
					failCount := k.IncrementPublisherFailureStreak(ctx, website.Domain)
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

func (k Keeper) eligibleVerifierAddrs(ctx sdk.Context) ([]string, error) {
	if k.verifiers == nil {
		return nil, nil
	}
	vparams := k.verifiers.GetParams(ctx)
	all := k.verifiers.ListVerifiers(ctx)
	eligible := make([]string, 0, len(all))
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
	}
	sort.Strings(eligible)
	return eligible, nil
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

func selectDeterministic(candidates []string, k int, seed []byte) []string {
	if k <= 0 || len(candidates) == 0 {
		return nil
	}
	if len(candidates) <= k {
		out := append([]string(nil), candidates...)
		return out
	}
	type scored struct {
		addr string
		hash [32]byte
	}
	scoredList := make([]scored, 0, len(candidates))
	for _, addr := range candidates {
		buf := make([]byte, 0, len(seed)+len(addr))
		buf = append(buf, seed...)
		buf = append(buf, []byte(addr)...)
		h := sha256.Sum256(buf)
		scoredList = append(scoredList, scored{addr: addr, hash: h})
	}
	sort.Slice(scoredList, func(i, j int) bool {
		cmp := bytes.Compare(scoredList[i].hash[:], scoredList[j].hash[:])
		if cmp == 0 {
			return scoredList[i].addr < scoredList[j].addr
		}
		return cmp < 0
	})
	out := make([]string, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, scoredList[i].addr)
	}
	return out
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

	eligible := make([]string, 0, len(submissions))
	for _, sub := range submissions {
		if sub.Passed {
			eligible = append(eligible, sub.Verifier)
		}
	}
	if len(eligible) == 0 {
		return k.tokenomics.BurnFromPool(ctx, sdk.NewCoins(sdk.NewCoin(verificationRewardDenom, verifierPoolPerAssignment)))
	}

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

	type weightedVerifier struct {
		addr   string
		weight sdkmath.Int
	}
	weighted := make([]weightedVerifier, 0, len(eligible))
	totalWeight := sdkmath.ZeroInt()
	for _, addr := range eligible {
		stake := bondByAddr[addr]
		if !stake.IsPositive() {
			continue
		}
		refActive := k.CountActiveReferredPublishers(ctx, addr)
		refFactor := int64(refActive)
		if refFactor <= 0 {
			refFactor = 1
		}
		weight := stake.MulRaw(refFactor)
		if !weight.IsPositive() {
			continue
		}
		totalWeight = totalWeight.Add(weight)
		weighted = append(weighted, weightedVerifier{addr: addr, weight: weight})
	}

	if len(weighted) == 0 || totalWeight.IsZero() {
		return k.tokenomics.BurnFromPool(ctx, sdk.NewCoins(sdk.NewCoin(verificationRewardDenom, verifierPoolPerAssignment)))
	}

	remaining := verifierPoolPerAssignment
	for i, entry := range weighted {
		acc, err := sdk.AccAddressFromBech32(entry.addr)
		if err != nil {
			return err
		}
		share := sdkmath.ZeroInt()
		if i == len(weighted)-1 {
			share = remaining
		} else {
			share = verifierPoolPerAssignment.Mul(entry.weight).Quo(totalWeight)
			if share.GT(remaining) {
				share = remaining
			}
		}
		if share.IsZero() {
			continue
		}
		remaining = remaining.Sub(share)
		coin := sdk.NewCoin(verificationRewardDenom, share)
		if err := k.tokenomics.SendFromPool(ctx, acc, sdk.NewCoins(coin)); err != nil {
			return err
		}
		if remaining.IsZero() {
			break
		}
	}
	if remaining.IsPositive() {
		if err := k.tokenomics.BurnFromPool(ctx, sdk.NewCoins(sdk.NewCoin(verificationRewardDenom, remaining))); err != nil {
			return err
		}
	}
	return nil
}
