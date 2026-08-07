package registry

import (
	"sort"
)

const (
	// SimilarTopN is the expected number of similar domains publishers embed.
	SimilarTopN = int32(15)
)

type similarRoundStats struct {
	MajorityExpectedHash string
	Passes               int
	Fails                int
	Quorum               int

	// VerifiedSimilarDomains is derived from the median matched count among passing submissions
	// that agree on the majority expected hash.
	VerifiedSimilarDomains int32
}

func computeSimilarRoundStats(assigned int, subs []PublisherVerificationSubmission) similarRoundStats {
	st := similarRoundStats{Quorum: requiredQuorum(assigned)}
	if len(subs) == 0 {
		return st
	}

	// Badge/owner verification alone determines whether the publisher is active.
	// Similar-site observations only determine the payout multiplier.
	passes, fails := 0, 0
	for _, s := range subs {
		if s.Passed {
			passes++
		} else {
			fails++
		}
	}
	st.Passes, st.Fails = passes, fails

	// Determine the majority expected set among passing submissions. Indexes may
	// contain fewer than SimilarTopN publishers during network bootstrap, so the
	// actual expected count is intentionally not required to equal SimilarTopN.
	hashCounts := map[string]int{}
	for _, s := range subs {
		if !s.Passed {
			continue
		}
		if s.ExpectedSetHash == "" {
			continue
		}
		hashCounts[s.ExpectedSetHash]++
	}
	bestHash := ""
	bestCount := 0
	for h, c := range hashCounts {
		if c > bestCount || (c == bestCount && h < bestHash) {
			bestHash = h
			bestCount = c
		}
	}
	if bestCount < st.Quorum {
		bestHash = ""
	}
	st.MajorityExpectedHash = bestHash

	// Use the median matched count from passing verifiers that agree on the
	// expected set. Similar-link disagreement never changes badge verification.
	passMatched := make([]int, 0)
	for _, s := range subs {
		if !s.Passed || bestHash == "" {
			continue
		}
		if s.ExpectedSetHash != bestHash {
			continue
		}
		passMatched = append(passMatched, int(s.MatchedSimilarDomains))
	}

	if len(passMatched) == 0 {
		st.VerifiedSimilarDomains = 0
		return st
	}
	// Median matched count.
	sort.Ints(passMatched)
	med := passMatched[len(passMatched)/2]
	if med < 0 {
		med = 0
	}
	if med > int(SimilarTopN) {
		med = int(SimilarTopN)
	}
	st.VerifiedSimilarDomains = int32(med)
	return st
}
