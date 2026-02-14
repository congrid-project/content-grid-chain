package registry

import (
	"math"
	"sort"
)

const (
	// SimilarTopN is the expected number of similar domains publishers embed.
	SimilarTopN = int32(15)
	// SimilarOverlapThreshold is the minimum overlap ratio (matched/expected) to consider consistent.
	// With top-15, 0.6 means at least 9 matches.
	SimilarOverlapThreshold = 0.6
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

	// If no submission includes similar fields, fall back to legacy behaviour.
	anySimilar := false
	for _, s := range subs {
		if s.ExpectedSimilarDomains > 0 {
			anySimilar = true
			break
		}
	}
	if !anySimilar {
		passes, fails := 0, 0
		for _, s := range subs {
			if s.Passed {
				passes++
			} else {
				fails++
			}
		}
		st.Passes, st.Fails = passes, fails
		return st
	}

	// Determine the majority expected hash among submissions that claim to have top-N.
	hashCounts := map[string]int{}
	for _, s := range subs {
		if s.ExpectedSimilarDomains != SimilarTopN {
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
	st.MajorityExpectedHash = bestHash

	// Count passes/fails using overlap threshold and majority expected hash.
	passMatched := make([]int, 0)
	passes, fails := 0, 0
	for _, s := range subs {
		ok := s.Passed
		if !ok {
			fails++
			continue
		}
		// Similar constraints
		if s.ExpectedSimilarDomains != SimilarTopN {
			fails++
			continue
		}
		if bestHash != "" && s.ExpectedSetHash != bestHash {
			fails++
			continue
		}
		minMatches := int32(math.Ceil(float64(SimilarTopN) * SimilarOverlapThreshold))
		if s.MatchedSimilarDomains < minMatches {
			fails++
			continue
		}
		passes++
		passMatched = append(passMatched, int(s.MatchedSimilarDomains))
	}
	st.Passes, st.Fails = passes, fails

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
