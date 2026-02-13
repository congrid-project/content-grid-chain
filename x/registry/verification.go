package registry

import (
	"encoding/hex"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/registry/typespb"
)

// PublisherVerificationAssignment captures the on-chain assignment window for a publisher.
type PublisherVerificationAssignment struct {
	RoundStartUnix  int64    `json:"round_start_unix"`
	Domain          string   `json:"domain"`
	StartAtUnix     int64    `json:"start_at_unix"`
	DeadlineUnix    int64    `json:"deadline_unix"`
	Verifiers       []string `json:"verifiers"`
	Finalized       bool     `json:"finalized"`
	FinalizedAtUnix int64    `json:"finalized_at_unix"`
}

func (a PublisherVerificationAssignment) ValidateBasic() error {
	if a.RoundStartUnix <= 0 {
		return fmt.Errorf("round start unix must be positive")
	}
	if strings.TrimSpace(a.Domain) == "" {
		return fmt.Errorf("domain required")
	}
	if a.StartAtUnix <= 0 {
		return fmt.Errorf("start at unix must be positive")
	}
	if a.DeadlineUnix <= 0 {
		return fmt.Errorf("deadline unix must be positive")
	}
	if a.DeadlineUnix < a.StartAtUnix {
		return fmt.Errorf("deadline must be >= start at")
	}
	for _, v := range a.Verifiers {
		if _, err := sdk.AccAddressFromBech32(strings.TrimSpace(v)); err != nil {
			return fmt.Errorf("invalid verifier address: %w", err)
		}
	}
	return nil
}

func (a PublisherVerificationAssignment) ToProto() *typespb.PublisherVerificationAssignment {
	return &typespb.PublisherVerificationAssignment{
		RoundStartUnix:  a.RoundStartUnix,
		Domain:          a.Domain,
		StartAtUnix:     a.StartAtUnix,
		DeadlineUnix:    a.DeadlineUnix,
		Verifiers:       append([]string(nil), a.Verifiers...),
		Finalized:       a.Finalized,
		FinalizedAtUnix: a.FinalizedAtUnix,
	}
}

func PublisherVerificationAssignmentFromProto(pb *typespb.PublisherVerificationAssignment) (PublisherVerificationAssignment, error) {
	if pb == nil {
		return PublisherVerificationAssignment{}, fmt.Errorf("nil assignment")
	}
	out := PublisherVerificationAssignment{
		RoundStartUnix:  pb.GetRoundStartUnix(),
		Domain:          NormalizeDomain(pb.GetDomain()),
		StartAtUnix:     pb.GetStartAtUnix(),
		DeadlineUnix:    pb.GetDeadlineUnix(),
		Verifiers:       append([]string(nil), pb.GetVerifiers()...),
		Finalized:       pb.GetFinalized(),
		FinalizedAtUnix: pb.GetFinalizedAtUnix(),
	}
	return out, out.ValidateBasic()
}

// VerificationRoundMeta records deterministic inputs used when creating a verification round.
type VerificationRoundMeta struct {
	RoundStartUnix            int64  `json:"round_start_unix"`
	SeedHex                   string `json:"seed_hex"`
	RoundIntervalSeconds      int64  `json:"round_interval_seconds"`
	AssignmentDelayMaxSeconds int64  `json:"assignment_delay_max_seconds"`
	CreatedAtUnix             int64  `json:"created_at_unix"`
	AnchorHeight              int64  `json:"anchor_height"`
	AnchorHashHex             string `json:"anchor_hash_hex"`
	VerifierSetHash           string `json:"verifier_set_hash"`
	VerifierSetSize           int32  `json:"verifier_set_size"`
}

func (m VerificationRoundMeta) ValidateBasic() error {
	if m.RoundStartUnix <= 0 {
		return fmt.Errorf("round start unix must be positive")
	}
	if strings.TrimSpace(m.SeedHex) == "" {
		return fmt.Errorf("seed_hex required")
	}
	if _, err := hex.DecodeString(strings.TrimSpace(m.SeedHex)); err != nil {
		return fmt.Errorf("invalid seed_hex: %w", err)
	}
	if m.RoundIntervalSeconds <= 0 {
		return fmt.Errorf("round_interval_seconds must be positive")
	}
	if m.AssignmentDelayMaxSeconds <= 0 {
		return fmt.Errorf("assignment_delay_max_seconds must be positive")
	}
	if m.AssignmentDelayMaxSeconds > m.RoundIntervalSeconds {
		return fmt.Errorf("assignment_delay_max_seconds must be <= round_interval_seconds")
	}
	if m.CreatedAtUnix <= 0 {
		return fmt.Errorf("created_at_unix must be positive")
	}
	if m.AnchorHeight < 0 {
		return fmt.Errorf("anchor_height must be >= 0")
	}
	if strings.TrimSpace(m.AnchorHashHex) != "" {
		if _, err := hex.DecodeString(strings.TrimSpace(m.AnchorHashHex)); err != nil {
			return fmt.Errorf("invalid anchor_hash_hex: %w", err)
		}
	}
	if strings.TrimSpace(m.VerifierSetHash) != "" {
		if _, err := hex.DecodeString(strings.TrimSpace(m.VerifierSetHash)); err != nil {
			return fmt.Errorf("invalid verifier_set_hash: %w", err)
		}
	}
	if m.VerifierSetSize < 0 {
		return fmt.Errorf("verifier_set_size must be >= 0")
	}
	return nil
}

func (m VerificationRoundMeta) ToProto() *typespb.VerificationRoundMeta {
	return &typespb.VerificationRoundMeta{
		RoundStartUnix:            m.RoundStartUnix,
		SeedHex:                   m.SeedHex,
		RoundIntervalSeconds:      m.RoundIntervalSeconds,
		AssignmentDelayMaxSeconds: m.AssignmentDelayMaxSeconds,
		CreatedAtUnix:             m.CreatedAtUnix,
		AnchorHeight:              m.AnchorHeight,
		AnchorHashHex:             m.AnchorHashHex,
		VerifierSetHash:           m.VerifierSetHash,
		VerifierSetSize:           m.VerifierSetSize,
	}
}

func VerificationRoundMetaFromProto(pb *typespb.VerificationRoundMeta) (VerificationRoundMeta, error) {
	if pb == nil {
		return VerificationRoundMeta{}, fmt.Errorf("nil round meta")
	}
	out := VerificationRoundMeta{
		RoundStartUnix:            pb.GetRoundStartUnix(),
		SeedHex:                   strings.TrimSpace(pb.GetSeedHex()),
		RoundIntervalSeconds:      pb.GetRoundIntervalSeconds(),
		AssignmentDelayMaxSeconds: pb.GetAssignmentDelayMaxSeconds(),
		CreatedAtUnix:             pb.GetCreatedAtUnix(),
		AnchorHeight:              pb.GetAnchorHeight(),
		AnchorHashHex:             strings.TrimSpace(pb.GetAnchorHashHex()),
		VerifierSetHash:           strings.TrimSpace(pb.GetVerifierSetHash()),
		VerifierSetSize:           pb.GetVerifierSetSize(),
	}
	return out, out.ValidateBasic()
}

// PublisherVerificationSubmission is a verifier's result for an assignment window.
type PublisherVerificationSubmission struct {
	RoundStartUnix  int64  `json:"round_start_unix"`
	Domain          string `json:"domain"`
	Verifier        string `json:"verifier"`
	Passed          bool   `json:"passed"`
	ObservedAtUnix  int64  `json:"observed_at_unix"`
	LatencyMs       int64  `json:"latency_ms"`
	SubmittedAtUnix int64  `json:"submitted_at_unix"`

	// Similar-site verification metrics (domain-only, no paths).
	ObservedSimilarDomains int32  `json:"observed_similar_domains"`
	MatchedSimilarDomains  int32  `json:"matched_similar_domains"`
	ExpectedSimilarDomains int32  `json:"expected_similar_domains"`
	ExpectedSetHash        string `json:"expected_set_hash"`
	ObservedSetHash        string `json:"observed_set_hash"`
}

func (s PublisherVerificationSubmission) ValidateBasic() error {
	if s.RoundStartUnix <= 0 {
		return fmt.Errorf("round start unix must be positive")
	}
	if strings.TrimSpace(s.Domain) == "" {
		return fmt.Errorf("domain required")
	}
	if strings.TrimSpace(s.Verifier) == "" {
		return fmt.Errorf("verifier required")
	}
	if _, err := sdk.AccAddressFromBech32(strings.TrimSpace(s.Verifier)); err != nil {
		return fmt.Errorf("invalid verifier address: %w", err)
	}
	if s.ObservedAtUnix <= 0 {
		return fmt.Errorf("observed at unix must be positive")
	}
	if s.LatencyMs < 0 {
		return fmt.Errorf("latency ms must be >= 0")
	}
	if s.SubmittedAtUnix <= 0 {
		return fmt.Errorf("submitted at unix must be positive")
	}
	if s.ObservedSimilarDomains < 0 || s.MatchedSimilarDomains < 0 || s.ExpectedSimilarDomains < 0 {
		return fmt.Errorf("similar domain counts must be >= 0")
	}
	if s.MatchedSimilarDomains > s.ExpectedSimilarDomains {
		return fmt.Errorf("matched_similar_domains cannot exceed expected_similar_domains")
	}
	if s.ExpectedSimilarDomains > 0 {
		if strings.TrimSpace(s.ExpectedSetHash) == "" {
			return fmt.Errorf("expected_set_hash required when expected_similar_domains > 0")
		}
		if strings.TrimSpace(s.ObservedSetHash) == "" {
			return fmt.Errorf("observed_set_hash required when expected_similar_domains > 0")
		}
	}
	return nil
}

func (s PublisherVerificationSubmission) ToProto() *typespb.PublisherVerificationSubmission {
	return &typespb.PublisherVerificationSubmission{
		RoundStartUnix:         s.RoundStartUnix,
		Domain:                 s.Domain,
		Verifier:               s.Verifier,
		Passed:                 s.Passed,
		ObservedAtUnix:         s.ObservedAtUnix,
		LatencyMs:              s.LatencyMs,
		SubmittedAtUnix:        s.SubmittedAtUnix,
		ObservedSimilarDomains: s.ObservedSimilarDomains,
		MatchedSimilarDomains:  s.MatchedSimilarDomains,
		ExpectedSimilarDomains: s.ExpectedSimilarDomains,
		ExpectedSetHash:        s.ExpectedSetHash,
		ObservedSetHash:        s.ObservedSetHash,
	}
}

func PublisherVerificationSubmissionFromProto(pb *typespb.PublisherVerificationSubmission) (PublisherVerificationSubmission, error) {
	if pb == nil {
		return PublisherVerificationSubmission{}, fmt.Errorf("nil submission")
	}
	out := PublisherVerificationSubmission{
		RoundStartUnix:         pb.GetRoundStartUnix(),
		Domain:                 NormalizeDomain(pb.GetDomain()),
		Verifier:               strings.TrimSpace(pb.GetVerifier()),
		Passed:                 pb.GetPassed(),
		ObservedAtUnix:         pb.GetObservedAtUnix(),
		LatencyMs:              pb.GetLatencyMs(),
		SubmittedAtUnix:        pb.GetSubmittedAtUnix(),
		ObservedSimilarDomains: pb.GetObservedSimilarDomains(),
		MatchedSimilarDomains:  pb.GetMatchedSimilarDomains(),
		ExpectedSimilarDomains: pb.GetExpectedSimilarDomains(),
		ExpectedSetHash:        strings.TrimSpace(pb.GetExpectedSetHash()),
		ObservedSetHash:        strings.TrimSpace(pb.GetObservedSetHash()),
	}
	return out, out.ValidateBasic()
}

func assignmentHasVerifier(assignment PublisherVerificationAssignment, verifier string) bool {
	for _, v := range assignment.Verifiers {
		if v == verifier {
			return true
		}
	}
	return false
}
