package registry

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strings"

	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (k Keeper) SetLastRoundStart(ctx sdk.Context, roundStartUnix int64) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), verificationMetaPrefix)
	bz := make([]byte, 8)
	binary.BigEndian.PutUint64(bz, uint64(roundStartUnix))
	store.Set(lastRoundStartKey, bz)
}

func (k Keeper) GetLastRoundStart(ctx sdk.Context) (int64, bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), verificationMetaPrefix)
	bz := store.Get(lastRoundStartKey)
	if len(bz) == 0 {
		return 0, false
	}
	return int64(binary.BigEndian.Uint64(bz)), true
}

func (k Keeper) SetRoundMeta(ctx sdk.Context, meta VerificationRoundMeta) error {
	if err := meta.ValidateBasic(); err != nil {
		return err
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), verificationMetaPrefix)
	bz, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	store.Set(roundMetaKey(meta.RoundStartUnix), bz)
	return nil
}

func (k Keeper) GetRoundMeta(ctx sdk.Context, roundStartUnix int64) (VerificationRoundMeta, bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), verificationMetaPrefix)
	bz := store.Get(roundMetaKey(roundStartUnix))
	if len(bz) == 0 {
		return VerificationRoundMeta{}, false
	}
	var meta VerificationRoundMeta
	if err := json.Unmarshal(bz, &meta); err != nil {
		panic(fmt.Errorf("failed to decode round meta: %w", err))
	}
	return meta, true
}

func (k Keeper) SetDrandBeacon(ctx sdk.Context, beacon DrandBeacon) error {
	if err := beacon.ValidateBasic(); err != nil {
		return err
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), drandStorePrefix)
	key := drandBeaconKey(beacon.Round)
	if store.Has(key) {
		return fmt.Errorf("drand beacon for round %d already exists", beacon.Round)
	}
	bz, err := json.Marshal(beacon)
	if err != nil {
		return err
	}
	store.Set(key, bz)

	latestRound := uint64(0)
	if lbz := store.Get(drandLatestRoundKey); len(lbz) == 8 {
		latestRound = binary.BigEndian.Uint64(lbz)
	}
	if beacon.Round >= latestRound {
		rbz := make([]byte, 8)
		binary.BigEndian.PutUint64(rbz, beacon.Round)
		store.Set(drandLatestRoundKey, rbz)
	}
	return nil
}

func (k Keeper) GetDrandBeacon(ctx sdk.Context, round uint64) (DrandBeacon, bool) {
	if round == 0 {
		return DrandBeacon{}, false
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), drandStorePrefix)
	bz := store.Get(drandBeaconKey(round))
	if len(bz) == 0 {
		return DrandBeacon{}, false
	}
	var beacon DrandBeacon
	if err := json.Unmarshal(bz, &beacon); err != nil {
		panic(fmt.Errorf("failed to decode drand beacon: %w", err))
	}
	return beacon, true
}

func (k Keeper) GetLatestDrandBeacon(ctx sdk.Context) (DrandBeacon, bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), drandStorePrefix)
	lbz := store.Get(drandLatestRoundKey)
	if len(lbz) != 8 {
		return DrandBeacon{}, false
	}
	round := binary.BigEndian.Uint64(lbz)
	if round == 0 {
		return DrandBeacon{}, false
	}
	return k.GetDrandBeacon(ctx, round)
}

func (k Keeper) SetAssignment(ctx sdk.Context, assignment PublisherVerificationAssignment) error {
	assignment.Domain = NormalizeDomain(assignment.Domain)
	if err := assignment.ValidateBasic(); err != nil {
		return err
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), assignmentStorePrefix)
	bz, err := json.Marshal(assignment)
	if err != nil {
		return err
	}
	store.Set(assignmentKey(assignment.RoundStartUnix, assignment.Domain), bz)
	if !assignment.RewardsSettled {
		k.markVerificationRoundUnsettled(ctx, assignment.RoundStartUnix)
	}
	return nil
}

func (k Keeper) GetAssignment(ctx sdk.Context, roundStartUnix int64, domain string) (PublisherVerificationAssignment, bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), assignmentStorePrefix)
	bz := store.Get(assignmentKey(roundStartUnix, NormalizeDomain(domain)))
	if len(bz) == 0 {
		return PublisherVerificationAssignment{}, false
	}
	var assignment PublisherVerificationAssignment
	if err := json.Unmarshal(bz, &assignment); err != nil {
		panic(fmt.Errorf("failed to decode assignment: %w", err))
	}
	return assignment, true
}

func (k Keeper) IterateAssignments(ctx sdk.Context, cb func(PublisherVerificationAssignment) bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), assignmentStorePrefix)
	iter := store.Iterator(nil, nil)
	defer iter.Close()
	for ; iter.Valid(); iter.Next() {
		var assignment PublisherVerificationAssignment
		if err := json.Unmarshal(iter.Value(), &assignment); err != nil {
			panic(fmt.Errorf("failed to decode assignment: %w", err))
		}
		if stop := cb(assignment); stop {
			return
		}
	}
}

func (k Keeper) ListAssignmentsForVerifier(ctx sdk.Context, verifier string, includeFinalized bool) []PublisherVerificationAssignment {
	var out []PublisherVerificationAssignment
	k.IterateAssignments(ctx, func(a PublisherVerificationAssignment) bool {
		if !includeFinalized && a.Finalized {
			return false
		}
		if assignmentHasVerifier(a, verifier) {
			out = append(out, a)
		}
		return false
	})
	return out
}

func (k Keeper) CountAssignmentsForRound(ctx sdk.Context, roundStartUnix int64) int {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), assignmentStorePrefix)
	roundPrefix := make([]byte, 8)
	binary.BigEndian.PutUint64(roundPrefix, uint64(roundStartUnix))
	iter := store.Iterator(roundPrefix, storetypes.PrefixEndBytes(roundPrefix))
	defer iter.Close()
	count := 0
	for ; iter.Valid(); iter.Next() {
		count++
	}
	return count
}

func (k Keeper) ListAssignmentsForRound(ctx sdk.Context, roundStartUnix int64) []PublisherVerificationAssignment {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), assignmentStorePrefix)
	roundPrefix := verificationRoundKey(roundStartUnix)
	iter := store.Iterator(roundPrefix, storetypes.PrefixEndBytes(roundPrefix))
	defer iter.Close()

	assignments := make([]PublisherVerificationAssignment, 0)
	for ; iter.Valid(); iter.Next() {
		var assignment PublisherVerificationAssignment
		if err := json.Unmarshal(iter.Value(), &assignment); err != nil {
			panic(fmt.Errorf("failed to decode assignment: %w", err))
		}
		assignments = append(assignments, assignment)
	}
	return assignments
}

func (k Keeper) markVerificationRoundUnsettled(ctx sdk.Context, roundStartUnix int64) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), unsettledRoundStorePrefix)
	store.Set(verificationRoundKey(roundStartUnix), []byte{1})
}

func (k Keeper) clearVerificationRoundUnsettled(ctx sdk.Context, roundStartUnix int64) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), unsettledRoundStorePrefix)
	store.Delete(verificationRoundKey(roundStartUnix))
}

func (k Keeper) listUnsettledVerificationRounds(ctx sdk.Context) []int64 {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), unsettledRoundStorePrefix)
	iter := store.Iterator(nil, nil)
	defer iter.Close()

	rounds := make([]int64, 0)
	for ; iter.Valid(); iter.Next() {
		if len(iter.Key()) != 8 {
			panic(fmt.Errorf("invalid unsettled verification round key length %d", len(iter.Key())))
		}
		rounds = append(rounds, int64(binary.BigEndian.Uint64(iter.Key())))
	}
	return rounds
}

func (k Keeper) CountActiveReferredPublishers(ctx sdk.Context, verifier string) int {
	verifier = strings.TrimSpace(verifier)
	count := 0
	k.IterateWebsites(ctx, func(w Website) bool {
		if w.Status != StatusVerified {
			return false
		}
		if strings.TrimSpace(w.Referrer) == verifier {
			count++
		}
		return false
	})
	return count
}

func (k Keeper) SetCommit(ctx sdk.Context, roundStartUnix int64, domain, verifier, commitHash string) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), commitStorePrefix)
	key := commitKey(roundStartUnix, NormalizeDomain(domain), verifier)
	store.Set(key, []byte(commitHash))
}

func (k Keeper) GetCommit(ctx sdk.Context, roundStartUnix int64, domain, verifier string) (string, bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), commitStorePrefix)
	key := commitKey(roundStartUnix, NormalizeDomain(domain), verifier)
	bz := store.Get(key)
	if len(bz) == 0 {
		return "", false
	}
	return string(bz), true
}

func (k Keeper) DeleteCommit(ctx sdk.Context, roundStartUnix int64, domain, verifier string) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), commitStorePrefix)
	key := commitKey(roundStartUnix, NormalizeDomain(domain), verifier)
	store.Delete(key)
}

func (k Keeper) SetSubmission(ctx sdk.Context, submission PublisherVerificationSubmission) error {
	submission.Domain = NormalizeDomain(submission.Domain)
	if err := submission.ValidateBasic(); err != nil {
		return err
	}
	store := prefix.NewStore(ctx.KVStore(k.storeKey), submissionStorePrefix)
	bz, err := json.Marshal(submission)
	if err != nil {
		return err
	}
	store.Set(submissionKey(submission.RoundStartUnix, submission.Domain, submission.Verifier), bz)
	return nil
}

type verifierPenaltyState struct {
	Count              int32 `json:"count"`
	SuspendedUntilUnix int64 `json:"suspended_until_unix"`
}

func (k Keeper) GetPublisherFailureStreak(ctx sdk.Context, domain string) int32 {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), publisherFailStorePrefix)
	bz := store.Get([]byte(NormalizeDomain(domain)))
	if len(bz) != 4 {
		return 0
	}
	return int32(binary.BigEndian.Uint32(bz))
}

func (k Keeper) SetPublisherFailureStreak(ctx sdk.Context, domain string, count int32) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), publisherFailStorePrefix)
	key := []byte(NormalizeDomain(domain))
	if count <= 0 {
		store.Delete(key)
		return
	}
	bz := make([]byte, 4)
	binary.BigEndian.PutUint32(bz, uint32(count))
	store.Set(key, bz)
}

func (k Keeper) IncrementPublisherFailureStreak(ctx sdk.Context, domain string) int32 {
	next := k.GetPublisherFailureStreak(ctx, domain) + 1
	if next < 0 {
		next = 0
	}
	k.SetPublisherFailureStreak(ctx, domain, next)
	return next
}

func (k Keeper) ClearPublisherFailureStreak(ctx sdk.Context, domain string) {
	k.SetPublisherFailureStreak(ctx, domain, 0)
}

func (k Keeper) GetVerifierPenalty(ctx sdk.Context, verifier string) verifierPenaltyState {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), verifierPenaltyPrefix)
	bz := store.Get([]byte(verifier))
	if len(bz) == 0 {
		return verifierPenaltyState{}
	}
	var state verifierPenaltyState
	if err := json.Unmarshal(bz, &state); err != nil {
		panic(fmt.Errorf("failed to decode verifier penalty state: %w", err))
	}
	return state
}

func (k Keeper) SetVerifierPenalty(ctx sdk.Context, verifier string, state verifierPenaltyState) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), verifierPenaltyPrefix)
	key := []byte(verifier)
	if state.Count <= 0 && state.SuspendedUntilUnix <= 0 {
		store.Delete(key)
		return
	}
	bz, err := json.Marshal(state)
	if err != nil {
		panic(fmt.Errorf("failed to encode verifier penalty state: %w", err))
	}
	store.Set(key, bz)
}

func (k Keeper) ClearVerifierPenalty(ctx sdk.Context, verifier string) {
	k.SetVerifierPenalty(ctx, verifier, verifierPenaltyState{})
}

func (k Keeper) ApplyVerifierPenalty(ctx sdk.Context, verifier string, nowUnix int64, roundIntervalSeconds int64, suspendThreshold int32, suspendRounds int64) verifierPenaltyState {
	state := k.GetVerifierPenalty(ctx, verifier)
	state.Count++
	if suspendThreshold <= 0 {
		suspendThreshold = 3
	}
	if suspendRounds <= 0 {
		suspendRounds = 3
	}
	if state.Count >= suspendThreshold {
		duration := roundIntervalSeconds * suspendRounds
		if duration < 60 {
			duration = 60
		}
		suspendedUntil := nowUnix + duration
		if suspendedUntil > state.SuspendedUntilUnix {
			state.SuspendedUntilUnix = suspendedUntil
		}
	}
	k.SetVerifierPenalty(ctx, verifier, state)
	return state
}

func (k Keeper) IsVerifierSuspended(ctx sdk.Context, verifier string, nowUnix int64) bool {
	state := k.GetVerifierPenalty(ctx, verifier)
	return state.SuspendedUntilUnix > nowUnix
}

func (k Keeper) GetSubmission(ctx sdk.Context, roundStartUnix int64, domain, verifier string) (PublisherVerificationSubmission, bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), submissionStorePrefix)
	bz := store.Get(submissionKey(roundStartUnix, NormalizeDomain(domain), verifier))
	if len(bz) == 0 {
		return PublisherVerificationSubmission{}, false
	}
	var submission PublisherVerificationSubmission
	if err := json.Unmarshal(bz, &submission); err != nil {
		panic(fmt.Errorf("failed to decode submission: %w", err))
	}
	return submission, true
}

func (k Keeper) ListSubmissions(ctx sdk.Context, roundStartUnix int64, domain string) []PublisherVerificationSubmission {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), submissionStorePrefix)
	prefixKey := submissionPrefix(roundStartUnix, NormalizeDomain(domain))
	iter := store.Iterator(prefixKey, storetypes.PrefixEndBytes(prefixKey))
	defer iter.Close()
	out := []PublisherVerificationSubmission{}
	for ; iter.Valid(); iter.Next() {
		var submission PublisherVerificationSubmission
		if err := json.Unmarshal(iter.Value(), &submission); err != nil {
			panic(fmt.Errorf("failed to decode submission: %w", err))
		}
		out = append(out, submission)
	}
	return out
}

func roundMetaKey(roundStartUnix int64) []byte {
	key := make([]byte, len(roundMetaKeyPrefix)+8)
	copy(key, roundMetaKeyPrefix)
	binary.BigEndian.PutUint64(key[len(roundMetaKeyPrefix):], uint64(roundStartUnix))
	return key
}

func drandBeaconKey(round uint64) []byte {
	key := make([]byte, len(drandBeaconKeyPrefix)+8)
	copy(key, drandBeaconKeyPrefix)
	binary.BigEndian.PutUint64(key[len(drandBeaconKeyPrefix):], round)
	return key
}

func assignmentKey(roundStartUnix int64, domain string) []byte {
	roundKey := verificationRoundKey(roundStartUnix)
	key := make([]byte, len(roundKey)+len(domain))
	copy(key, roundKey)
	copy(key[8:], []byte(domain))
	return key
}

func verificationRoundKey(roundStartUnix int64) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, uint64(roundStartUnix))
	return key
}

func submissionKey(roundStartUnix int64, domain, verifier string) []byte {
	prefixKey := submissionPrefix(roundStartUnix, domain)
	key := make([]byte, len(prefixKey)+len(verifier))
	copy(key, prefixKey)
	copy(key[len(prefixKey):], []byte(verifier))
	return key
}

func submissionPrefix(roundStartUnix int64, domain string) []byte {
	base := assignmentKey(roundStartUnix, domain)
	prefixKey := make([]byte, len(base)+1)
	copy(prefixKey, base)
	prefixKey[len(base)] = 0x00
	return prefixKey
}

func commitKey(roundStartUnix int64, domain, verifier string) []byte {
	prefixKey := submissionPrefix(roundStartUnix, domain)
	key := make([]byte, len(prefixKey)+len(verifier))
	copy(key, prefixKey)
	copy(key[len(prefixKey):], []byte(verifier))
	return key
}
