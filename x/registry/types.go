package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	typespb "content-grid-chain/x/registry/typespb"
)

// WebsiteStatus represents verification status of a website.
type WebsiteStatus int32

const (
	StatusUnspecified WebsiteStatus = 0
	StatusPending     WebsiteStatus = 1
	StatusVerified    WebsiteStatus = 2
	StatusRevoked     WebsiteStatus = 3
)

const (
	DefaultDrandSchemeID              = "bls-unchained-g1-rfc9380"
	DefaultDrandChainHash             = "52db9ba70e0cc0f6eaf7803dd07447a1f5477735fd3f661792ba94600c84e971"
	DefaultDrandPublicKeyHex          = "83cf0f2896adee7eb8b5f01fcad3912212c437e0073e911fb90022d3e760183c8c4b450b6a0a6c3ac6a5776a2d1064510d1fec758c921cc22b0e17e63aaf4bcb5ed66304de9cf809bd274ca73bab4af5a6e9c76a4bc09e76eae8991ef5ece45a"
	DefaultDrandGenesisTimeUnix int64 = 1_692_803_367
	DefaultDrandPeriodSeconds   int64 = 3
	DefaultDrandRoundOffsetSec  int64 = 60
)

func (s WebsiteStatus) String() string {
	switch s {
	case StatusUnspecified:
		return "UNSPECIFIED"
	case StatusPending:
		return "PENDING"
	case StatusVerified:
		return "VERIFIED"
	case StatusRevoked:
		return "REVOKED"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(s))
	}
}

// Website represents a registered domain with an owner and status.
type Website struct {
	Domain             string        `json:"domain"`
	Owner              string        `json:"owner"`
	Status             WebsiteStatus `json:"status"`
	MetadataURI        string        `json:"metadata_uri,omitempty"`
	Proof              string        `json:"proof,omitempty"`
	RegisteredAtHeight int64         `json:"registered_at_height,omitempty"`
	Verifier           string        `json:"verifier,omitempty"`
	Referrer           string        `json:"referrer,omitempty"`
	CooldownUntilUnix  int64         `json:"cooldown_until_unix,omitempty"`
	CooldownCount      int32         `json:"cooldown_count,omitempty"`
}

// NormalizeDomain lowercases and trims a domain string.
func NormalizeDomain(d string) string {
	return strings.TrimSpace(strings.ToLower(d))
}

// IsDomainFormatValid performs a fast format validation against the domain regex.
func IsDomainFormatValid(domain string) bool {
	norm := NormalizeDomain(domain)
	if norm == "" {
		return false
	}
	return domainRe.MatchString(norm)
}

// domainRe supports domains with optional ports (e.g., example.com:8080).
var domainRe = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}(?::\d+)?$`)

// GetPrimaryDomain extracts the primary domain (eTLD+1) using a simplified logic.
// It returns the last two segments of the domain (e.g., "example.com" from "sub.example.com").
// Note: This logic treats "co.uk" as a primary domain if registered first.
func GetPrimaryDomain(domain string) (string, error) {
	norm := NormalizeDomain(domain)
	if !IsDomainFormatValid(norm) {
		return "", fmt.Errorf("invalid domain format: %s", domain)
	}

	// Remove port if present
	host := norm
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		host = host[:idx]
	}

	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return host, nil
	}

	// Simple logic: last two segments
	return strings.Join(parts[len(parts)-2:], "."), nil
}

// ValidateWebsite performs basic validation on a Website record.
func ValidateWebsite(w Website) error {
	if w.Domain == "" {
		return errors.New("domain required")
	}
	norm := NormalizeDomain(w.Domain)
	if norm != w.Domain {
		return fmt.Errorf("domain must be normalized (got %q want %q)", w.Domain, norm)
	}
	if !domainRe.MatchString(w.Domain) {
		return fmt.Errorf("invalid domain: %s", w.Domain)
	}
	if w.Owner == "" {
		return errors.New("owner address required")
	}
	if _, err := sdk.AccAddressFromBech32(w.Owner); err != nil {
		return fmt.Errorf("invalid owner address: %w", err)
	}
	if w.MetadataURI != "" {
		if len(w.MetadataURI) > 512 {
			return errors.New("metadata uri too long")
		}
		if _, err := url.ParseRequestURI(w.MetadataURI); err != nil {
			return fmt.Errorf("invalid metadata uri: %w", err)
		}
	}
	if len(w.Proof) > 0 && len(w.Proof) > 256 {
		return errors.New("proof value too long")
	}
	if w.RegisteredAtHeight < 0 {
		return errors.New("registered height cannot be negative")
	}
	if w.Verifier != "" {
		if _, err := sdk.AccAddressFromBech32(w.Verifier); err != nil {
			return fmt.Errorf("invalid verifier address: %w", err)
		}
	}
	if w.CooldownUntilUnix < 0 {
		return errors.New("cooldown_until_unix cannot be negative")
	}
	if w.CooldownCount < 0 {
		return errors.New("cooldown_count cannot be negative")
	}
	if strings.TrimSpace(w.Referrer) != "" {
		if _, err := sdk.AccAddressFromBech32(strings.TrimSpace(w.Referrer)); err != nil {
			return fmt.Errorf("invalid referrer address: %w", err)
		}
	}
	if w.Status < StatusPending || w.Status > StatusRevoked {
		return fmt.Errorf("invalid status: %d", w.Status)
	}
	return nil
}

// GenesisState defines the registry module's genesis state.
type GenesisState struct {
	Websites []Website       `json:"websites"`
	Params   PublisherParams `json:"params"`
}

// DefaultGenesis returns an empty but valid genesis state.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Websites: []Website{},
		Params:   DefaultPublisherParams(),
	}
}

// Validate validates the genesis state.
func (gs GenesisState) Validate() error {
	seen := make(map[string]struct{})
	sort.Slice(gs.Websites, func(i, j int) bool { return gs.Websites[i].Domain < gs.Websites[j].Domain })
	for _, w := range gs.Websites {
		if err := ValidateWebsite(w); err != nil {
			return err
		}
		if _, ok := seen[w.Domain]; ok {
			return fmt.Errorf("duplicate domain in genesis: %s", w.Domain)
		}
		seen[w.Domain] = struct{}{}
	}
	return gs.Params.Validate()
}

// ToProto converts the Website struct into its protobuf representation.
func (w Website) ToProto() *typespb.Website {
	return &typespb.Website{
		Domain:             w.Domain,
		Owner:              w.Owner,
		Status:             typespb.WebsiteStatus(w.Status),
		MetadataUri:        w.MetadataURI,
		Proof:              w.Proof,
		RegisteredAtHeight: w.RegisteredAtHeight,
		Verifier:           w.Verifier,
		Referrer:           w.Referrer,
		CooldownUntilUnix:  w.CooldownUntilUnix,
		CooldownCount:      w.CooldownCount,
	}
}

// WebsiteFromProto converts a protobuf Website into the module representation.
func WebsiteFromProto(p *typespb.Website) Website {
	if p == nil {
		return Website{}
	}
	return Website{
		Domain:             p.GetDomain(),
		Owner:              p.GetOwner(),
		Status:             WebsiteStatus(p.GetStatus()),
		MetadataURI:        p.GetMetadataUri(),
		Proof:              p.GetProof(),
		RegisteredAtHeight: p.GetRegisteredAtHeight(),
		Verifier:           p.GetVerifier(),
		Referrer:           p.GetReferrer(),
		CooldownUntilUnix:  p.GetCooldownUntilUnix(),
		CooldownCount:      p.GetCooldownCount(),
	}
}

func marshalWebsite(w Website) ([]byte, error) {
	return json.Marshal(w)
}

func unmarshalWebsite(b []byte) (Website, error) {
	var w Website
	return w, json.Unmarshal(b, &w)
}

// PublisherParams configure registration, verification, and fee routing.
type PublisherParams struct {
	VerifierBond                       sdkmath.Int           `json:"verifier_bond"`
	VerificationTTL                    int64                 `json:"verification_ttl"`
	MinVerifierCount                   int                   `json:"min_verifier_count"`
	MinPublisherScore                  sdkmath.LegacyDec     `json:"min_publisher_score"`
	MinVerifierScore                   sdkmath.LegacyDec     `json:"min_verifier_score"`
	RewardWeights                      PublisherScoreWeights `json:"reward_weights"`
	RewardSplit                        PublisherRewardSplit  `json:"reward_split"`
	VerifierWeights                    VerifierRewardWeights `json:"verifier_weights"`
	CommitWindowSeconds                int64                 `json:"commit_window_seconds"`
	SubmissionWindowSeconds            int64                 `json:"submission_window_seconds"`
	RoundIntervalSeconds               int64                 `json:"round_interval_seconds"`
	AssignmentDelayMaxSeconds          int64                 `json:"assignment_delay_max_seconds"`
	CooldownBaseSeconds                int64                 `json:"cooldown_base_seconds"`
	PublisherVerificationReward        sdkmath.Int           `json:"publisher_verification_reward"`
	VerifierVerificationReward         sdkmath.Int           `json:"verifier_verification_reward"`
	VerifierRewardBaseShareBps         int64                 `json:"verifier_reward_base_share_bps"`
	RequiredExternalLinksForFullReward int32                 `json:"required_external_links_for_full_reward"`
	EmissionTotalSupply                sdkmath.Int           `json:"emission_total_supply"`
	OperatorReserveBps                 int64                 `json:"operator_reserve_bps"`
	PublisherEmissionBps               int64                 `json:"publisher_emission_bps"`
	VerifierEmissionBps                int64                 `json:"verifier_emission_bps"`
	EmissionDurationHours              int64                 `json:"emission_duration_hours"`
	PublisherRevokeFailureThreshold    int32                 `json:"publisher_revoke_failure_threshold"`
	VerifierPenaltySuspendThreshold    int32                 `json:"verifier_penalty_suspend_threshold"`
	VerifierPenaltySuspendRounds       int64                 `json:"verifier_penalty_suspend_rounds"`
	DrandEnabled                       bool                  `json:"drand_enabled"`
	DrandStrictMode                    bool                  `json:"drand_strict_mode"`
	DrandSchemeID                      string                `json:"drand_scheme_id"`
	DrandPublicKeyHex                  string                `json:"drand_public_key_hex"`
	DrandChainHash                     string                `json:"drand_chain_hash"`
	DrandGenesisTimeUnix               int64                 `json:"drand_genesis_time_unix"`
	DrandPeriodSeconds                 int64                 `json:"drand_period_seconds"`
	DrandRoundOffsetSeconds            int64                 `json:"drand_round_offset_seconds"`
}

// DefaultPublisherParams returns reference values aligned with the economic blueprint.
func DefaultPublisherParams() PublisherParams {
	return PublisherParams{
		VerifierBond:      sdkmath.NewInt(50_000_000), // 50 CONGRID
		VerificationTTL:   2_000,
		MinVerifierCount:  3,
		MinPublisherScore: mustNewDec("0.55"),
		MinVerifierScore:  mustNewDec("0.40"),
		RewardWeights:     DefaultPublisherScoreWeights(),
		RewardSplit: PublisherRewardSplit{
			PublisherShare: mustNewDec("0.70"),
			VerifierShare:  mustNewDec("0.25"),
			ProtocolShare:  mustNewDec("0.05"),
		},
		VerifierWeights: VerifierRewardWeights{
			Accuracy:  mustNewDec("0.50"),
			Coverage:  mustNewDec("0.20"),
			Latency:   mustNewDec("0.15"),
			Freshness: mustNewDec("0.15"),
		},
		CommitWindowSeconds:                300,    // 5 minutes
		SubmissionWindowSeconds:            600,    // 10 minutes
		RoundIntervalSeconds:               3600,   // 1 hour
		AssignmentDelayMaxSeconds:          3600,   // spread assignments across up to 1 hour
		CooldownBaseSeconds:                604800, // 7 days
		PublisherVerificationReward:        sdkmath.NewInt(1_000_000),
		VerifierVerificationReward:         sdkmath.NewInt(500_000),
		VerifierRewardBaseShareBps:         4000,
		RequiredExternalLinksForFullReward: 15,
		EmissionTotalSupply:                sdkmath.NewInt(1_000_000_000_000000), // 1B CONGRID in ucongrid
		OperatorReserveBps:                 4000,
		PublisherEmissionBps:               1000,
		VerifierEmissionBps:                5000,
		EmissionDurationHours:              100 * 365 * 24,
		PublisherRevokeFailureThreshold:    3,
		VerifierPenaltySuspendThreshold:    3,
		VerifierPenaltySuspendRounds:       3,
		DrandEnabled:                       true,
		DrandStrictMode:                    true,
		DrandSchemeID:                      DefaultDrandSchemeID,
		DrandPublicKeyHex:                  DefaultDrandPublicKeyHex,
		DrandChainHash:                     DefaultDrandChainHash,
		DrandGenesisTimeUnix:               DefaultDrandGenesisTimeUnix,
		DrandPeriodSeconds:                 DefaultDrandPeriodSeconds,
		DrandRoundOffsetSeconds:            DefaultDrandRoundOffsetSec,
	}
}

// Validate ensures the params are sane and self-consistent.
func (pp PublisherParams) Validate() error {

	if pp.VerifierBond.IsNegative() {
		return fmt.Errorf("verifier bond must be non-negative")
	}
	if pp.VerificationTTL <= 0 {
		return fmt.Errorf("verification ttl must be positive")
	}
	if pp.MinVerifierCount <= 0 {
		return fmt.Errorf("min verifier count must be positive")
	}
	if err := ensureUnitInterval(pp.MinPublisherScore, "min publisher score"); err != nil {
		return err
	}
	if err := ensureUnitInterval(pp.MinVerifierScore, "min verifier score"); err != nil {
		return err
	}
	if err := pp.RewardWeights.Validate(); err != nil {
		return err
	}
	if err := pp.RewardSplit.Validate(); err != nil {
		return err
	}
	if err := pp.VerifierWeights.Validate(); err != nil {
		return err
	}
	if pp.CommitWindowSeconds <= 0 {
		return fmt.Errorf("commit window seconds must be positive")
	}
	if pp.CommitWindowSeconds >= pp.VerificationTTL {
		return fmt.Errorf("commit window seconds must be < verification ttl")
	}
	if pp.SubmissionWindowSeconds <= 0 {
		return fmt.Errorf("submission window seconds must be positive")
	}
	if pp.RoundIntervalSeconds <= 0 {
		return fmt.Errorf("round interval seconds must be positive")
	}
	if pp.AssignmentDelayMaxSeconds <= 0 {
		return fmt.Errorf("assignment delay max seconds must be positive")
	}
	if pp.AssignmentDelayMaxSeconds > pp.RoundIntervalSeconds {
		return fmt.Errorf("assignment delay max seconds must be <= round interval seconds")
	}
	if pp.CooldownBaseSeconds <= 0 {
		return fmt.Errorf("cooldown base seconds must be positive")
	}

	if pp.CommitWindowSeconds >= pp.SubmissionWindowSeconds {
		return fmt.Errorf("commit window seconds must be < submission window seconds")
	}

	if pp.PublisherVerificationReward.IsNegative() {
		return fmt.Errorf("publisher verification reward must be >= 0")
	}
	if pp.VerifierVerificationReward.IsNegative() {
		return fmt.Errorf("verifier verification reward must be >= 0")
	}
	if pp.VerifierRewardBaseShareBps < 0 || pp.VerifierRewardBaseShareBps > 10000 {
		return fmt.Errorf("verifier reward base share bps must be within [0,10000]")
	}
	if pp.RequiredExternalLinksForFullReward < 0 {
		return fmt.Errorf("required external links for full reward must be >= 0")
	}
	if pp.EmissionTotalSupply.IsNegative() {
		return fmt.Errorf("emission total supply must be >= 0")
	}
	if pp.OperatorReserveBps < 0 || pp.OperatorReserveBps > 10000 {
		return fmt.Errorf("operator reserve bps must be within [0,10000]")
	}
	if pp.PublisherEmissionBps < 0 || pp.PublisherEmissionBps > 10000 {
		return fmt.Errorf("publisher emission bps must be within [0,10000]")
	}
	if pp.VerifierEmissionBps < 0 || pp.VerifierEmissionBps > 10000 {
		return fmt.Errorf("verifier emission bps must be within [0,10000]")
	}
	if pp.EmissionDurationHours < 0 {
		return fmt.Errorf("emission duration hours must be >= 0")
	}
	if pp.OperatorReserveBps+pp.PublisherEmissionBps+pp.VerifierEmissionBps > 10000 {
		return fmt.Errorf("operator reserve + publisher emission + verifier emission bps must be <= 10000")
	}
	if pp.PublisherRevokeFailureThreshold < 0 {
		return fmt.Errorf("publisher revoke failure threshold must be >= 0")
	}
	if pp.VerifierPenaltySuspendThreshold < 0 {
		return fmt.Errorf("verifier penalty suspend threshold must be >= 0")
	}
	if pp.VerifierPenaltySuspendRounds < 0 {
		return fmt.Errorf("verifier penalty suspend rounds must be >= 0")
	}

	schemeID := strings.TrimSpace(pp.DrandSchemeID)
	if schemeID != "" && !isSupportedDrandSchemeID(schemeID) {
		return fmt.Errorf("unsupported drand scheme id: %s", schemeID)
	}
	pubKeyHex := pp.EffectiveDrandPublicKeyHex()
	if pubKeyHex != "" {
		if _, err := hex.DecodeString(pubKeyHex); err != nil {
			return fmt.Errorf("invalid drand public key hex: %w", err)
		}
	}
	chainHash := pp.EffectiveDrandChainHash()
	if chainHash != "" {
		decoded, err := hex.DecodeString(chainHash)
		if err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("drand_chain_hash must be 32-byte hex")
		}
	}
	if pp.DrandEnabled {
		if pubKeyHex == "" {
			return fmt.Errorf("drand_enabled requires drand_public_key_hex")
		}
		if chainHash == "" {
			return fmt.Errorf("drand_enabled requires drand_chain_hash")
		}
		if pp.EffectiveDrandGenesisTimeUnix() <= 0 {
			return fmt.Errorf("drand_enabled requires positive drand_genesis_time_unix")
		}
		if pp.EffectiveDrandPeriodSeconds() <= 0 {
			return fmt.Errorf("drand_enabled requires positive drand_period_seconds")
		}
		if pp.EffectiveDrandRoundOffsetSeconds() <= 0 {
			return fmt.Errorf("drand_enabled requires positive drand_round_offset_seconds")
		}
		if pp.EffectiveDrandRoundOffsetSeconds() > pp.RoundIntervalSeconds {
			return fmt.Errorf("drand_round_offset_seconds must be <= round_interval_seconds")
		}
	}
	return nil
}

func (pp PublisherParams) EffectiveRequiredExternalLinksForFullReward() int32 {
	if pp.RequiredExternalLinksForFullReward > 0 {
		return pp.RequiredExternalLinksForFullReward
	}
	return DefaultPublisherParams().RequiredExternalLinksForFullReward
}

func (pp PublisherParams) EffectiveVerifierRewardBaseShareBps() int64 {
	if pp.VerifierRewardBaseShareBps < 0 || pp.VerifierRewardBaseShareBps > 10000 {
		return DefaultPublisherParams().VerifierRewardBaseShareBps
	}
	return pp.VerifierRewardBaseShareBps
}

func (pp PublisherParams) EffectiveEmissionTotalSupply() sdkmath.Int {
	if pp.EmissionTotalSupply.IsPositive() {
		return pp.EmissionTotalSupply
	}
	return DefaultPublisherParams().EmissionTotalSupply
}

func (pp PublisherParams) EffectiveOperatorReserveBps() int64 {
	if pp.OperatorReserveBps > 0 {
		return pp.OperatorReserveBps
	}
	return DefaultPublisherParams().OperatorReserveBps
}

func (pp PublisherParams) EffectivePublisherEmissionBps() int64 {
	if pp.PublisherEmissionBps > 0 {
		return pp.PublisherEmissionBps
	}
	return DefaultPublisherParams().PublisherEmissionBps
}

func (pp PublisherParams) EffectiveVerifierEmissionBps() int64 {
	if pp.VerifierEmissionBps > 0 {
		return pp.VerifierEmissionBps
	}
	return DefaultPublisherParams().VerifierEmissionBps
}

func (pp PublisherParams) EffectiveEmissionDurationHours() int64 {
	if pp.EmissionDurationHours > 0 {
		return pp.EmissionDurationHours
	}
	return DefaultPublisherParams().EmissionDurationHours
}

func (pp PublisherParams) RoundEmissionPools(roundIntervalSeconds int64) (sdkmath.Int, sdkmath.Int, error) {
	totalSupply := pp.EffectiveEmissionTotalSupply()
	if !totalSupply.IsPositive() {
		return sdkmath.ZeroInt(), sdkmath.ZeroInt(), nil
	}
	publisherBps := pp.EffectivePublisherEmissionBps()
	verifierBps := pp.EffectiveVerifierEmissionBps()
	durationHours := pp.EffectiveEmissionDurationHours()
	if durationHours <= 0 {
		return sdkmath.ZeroInt(), sdkmath.ZeroInt(), fmt.Errorf("emission duration hours must be positive")
	}
	if roundIntervalSeconds <= 0 {
		roundIntervalSeconds = int64(time.Hour.Seconds())
	}
	denominator := durationHours * int64(time.Hour.Seconds()) * 10000
	if denominator <= 0 {
		return sdkmath.ZeroInt(), sdkmath.ZeroInt(), fmt.Errorf("invalid emission denominator")
	}

	publisherPool := sdkmath.ZeroInt()
	if publisherBps > 0 {
		publisherPool = totalSupply.MulRaw(publisherBps).MulRaw(roundIntervalSeconds).QuoRaw(denominator)
	}
	verifierPool := sdkmath.ZeroInt()
	if verifierBps > 0 {
		verifierPool = totalSupply.MulRaw(verifierBps).MulRaw(roundIntervalSeconds).QuoRaw(denominator)
	}
	return publisherPool, verifierPool, nil
}

func (pp PublisherParams) EffectivePublisherRevokeFailureThreshold() int32 {
	if pp.PublisherRevokeFailureThreshold > 0 {
		return pp.PublisherRevokeFailureThreshold
	}
	return DefaultPublisherParams().PublisherRevokeFailureThreshold
}

func (pp PublisherParams) EffectiveVerifierPenaltySuspendThreshold() int32 {
	if pp.VerifierPenaltySuspendThreshold > 0 {
		return pp.VerifierPenaltySuspendThreshold
	}
	return DefaultPublisherParams().VerifierPenaltySuspendThreshold
}

func (pp PublisherParams) EffectiveVerifierPenaltySuspendRounds() int64 {
	if pp.VerifierPenaltySuspendRounds > 0 {
		return pp.VerifierPenaltySuspendRounds
	}
	return DefaultPublisherParams().VerifierPenaltySuspendRounds
}

func (pp PublisherParams) EffectiveDrandEnabled() bool {
	return pp.DrandEnabled
}

func (pp PublisherParams) EffectiveDrandStrictMode() bool {
	// Drand delivery is always strict when enabled. Keeping the legacy field in
	// genesis JSON avoids breaking existing configurations while removing the
	// submit-or-fallback choice that could bias assignments.
	return pp.EffectiveDrandEnabled()
}

func (pp PublisherParams) EffectiveDrandSchemeID() string {
	if strings.TrimSpace(pp.DrandSchemeID) != "" {
		return strings.TrimSpace(pp.DrandSchemeID)
	}
	return DefaultPublisherParams().DrandSchemeID
}

func (pp PublisherParams) EffectiveDrandPublicKeyHex() string {
	if strings.TrimSpace(pp.DrandPublicKeyHex) != "" {
		return strings.TrimSpace(pp.DrandPublicKeyHex)
	}
	return DefaultDrandPublicKeyHex
}

func (pp PublisherParams) EffectiveDrandChainHash() string {
	if strings.TrimSpace(pp.DrandChainHash) != "" {
		return strings.ToLower(strings.TrimSpace(pp.DrandChainHash))
	}
	return DefaultDrandChainHash
}

func (pp PublisherParams) EffectiveDrandGenesisTimeUnix() int64 {
	if pp.DrandGenesisTimeUnix > 0 {
		return pp.DrandGenesisTimeUnix
	}
	return DefaultDrandGenesisTimeUnix
}

func (pp PublisherParams) EffectiveDrandPeriodSeconds() int64 {
	if pp.DrandPeriodSeconds > 0 {
		return pp.DrandPeriodSeconds
	}
	return DefaultDrandPeriodSeconds
}

func (pp PublisherParams) EffectiveDrandRoundOffsetSeconds() int64 {
	if pp.DrandRoundOffsetSeconds > 0 {
		return pp.DrandRoundOffsetSeconds
	}
	return DefaultDrandRoundOffsetSec
}

// WithStrictDrandEnabled fills missing quicknet metadata and enables the
// fail-closed exact-round behavior used by the drand-strict-v2 upgrade.
func (pp PublisherParams) WithStrictDrandEnabled() PublisherParams {
	pp.DrandSchemeID = pp.EffectiveDrandSchemeID()
	pp.DrandPublicKeyHex = pp.EffectiveDrandPublicKeyHex()
	pp.DrandChainHash = pp.EffectiveDrandChainHash()
	pp.DrandGenesisTimeUnix = pp.EffectiveDrandGenesisTimeUnix()
	pp.DrandPeriodSeconds = pp.EffectiveDrandPeriodSeconds()
	pp.DrandRoundOffsetSeconds = pp.EffectiveDrandRoundOffsetSeconds()
	pp.DrandEnabled = true
	pp.DrandStrictMode = true
	return pp
}

// PublisherRewardSplit controls how the publisher bucket is shared each epoch.
type PublisherRewardSplit struct {
	PublisherShare sdkmath.LegacyDec `json:"publisher_share"`
	VerifierShare  sdkmath.LegacyDec `json:"verifier_share"`
	ProtocolShare  sdkmath.LegacyDec `json:"protocol_share"`
}

// Validate ensures the split sums to one.
func (prs PublisherRewardSplit) Validate() error {
	shares := []sdkmath.LegacyDec{prs.PublisherShare, prs.VerifierShare, prs.ProtocolShare}
	return ensureSharesSumToOne(shares, "publisher reward split")
}

// VerifierRewardWeights capture how verifier KPIs convert to payout weights.
type VerifierRewardWeights struct {
	Accuracy  sdkmath.LegacyDec `json:"accuracy"`
	Coverage  sdkmath.LegacyDec `json:"coverage"`
	Latency   sdkmath.LegacyDec `json:"latency"`
	Freshness sdkmath.LegacyDec `json:"freshness"`
}

// Validate ensures weights sit in [0,1] and sum to one.
func (vw VerifierRewardWeights) Validate() error {
	weights := []sdkmath.LegacyDec{vw.Accuracy, vw.Coverage, vw.Latency, vw.Freshness}
	return ensureSharesSumToOne(weights, "verifier weight")
}

// PublisherScoreWeights parameterize the publisher reward calculation.
type PublisherScoreWeights struct {
	Availability sdkmath.LegacyDec `json:"availability"`
	Engagement   sdkmath.LegacyDec `json:"engagement"`
	Freshness    sdkmath.LegacyDec `json:"freshness"`
}

// DefaultPublisherScoreWeights tilts rewards toward uptime, then referral quality, then verification freshness.
func DefaultPublisherScoreWeights() PublisherScoreWeights {
	return PublisherScoreWeights{
		Availability: mustNewDec("0.50"),
		Engagement:   mustNewDec("0.30"),
		Freshness:    mustNewDec("0.20"),
	}
}

// Validate ensures weights form a proper distribution.
func (w PublisherScoreWeights) Validate() error {
	shares := []sdkmath.LegacyDec{w.Availability, w.Engagement, w.Freshness}
	return ensureSharesSumToOne(shares, "publisher weights")
}

// PublisherSnapshot represents the observed metrics for a site during a reward epoch.
type PublisherSnapshot struct {
	Domain               string `json:"domain"`
	VerifiedCongridLinks int    `json:"verified_congrid_links"`
	OnlineMinutes        int    `json:"online_minutes"`
	ExpectedMinutes      int    `json:"expected_minutes"`
	ReferralClicks       int    `json:"referral_clicks"`
	BaselineClicks       int    `json:"baseline_clicks"`
	LastVerifiedHeight   int64  `json:"last_verified_height"`
	CurrentHeight        int64  `json:"current_height"`
	VerificationTTL      int64  `json:"verification_ttl"`
}

// Score evaluates the snapshot against the provided weights.
func (ps PublisherSnapshot) Score(weights PublisherScoreWeights) (sdkmath.LegacyDec, error) {
	if err := weights.Validate(); err != nil {
		return sdkmath.LegacyDec{}, err
	}

	availability := ratioDec(ps.OnlineMinutes, ps.ExpectedMinutes)
	engagement := ratioDec(ps.ReferralClicks, maxInt(ps.BaselineClicks, 1))
	engagementCap := sdkmath.LegacyMustNewDecFromStr("3.0")
	if engagement.GT(engagementCap) {
		engagement = engagementCap
	}
	engagement = engagement.Quo(engagementCap)

	freshness := sdkmath.LegacyOneDec()
	if ps.VerificationTTL > 0 && ps.CurrentHeight > ps.LastVerifiedHeight {
		delta := ps.CurrentHeight - ps.LastVerifiedHeight
		if delta > ps.VerificationTTL {
			freshness = sdkmath.LegacyZeroDec()
		} else {
			freshness = sdkmath.LegacyNewDec(delta).QuoInt64(ps.VerificationTTL)
			freshness = sdkmath.LegacyOneDec().Sub(freshness)
		}
	}

	score := sdkmath.LegacyZeroDec()
	score = score.Add(weights.Availability.Mul(availability))
	score = score.Add(weights.Engagement.Mul(engagement))
	score = score.Add(weights.Freshness.Mul(freshness))

	return score, nil
}

// VerifierReport captures a worker's verification effort for a publisher snapshot.
type VerifierReport struct {
	Worker            string `json:"worker"`
	Checks            int    `json:"checks"`
	Matches           int    `json:"matches"`
	ExpectedChecks    int    `json:"expected_checks"`
	MedianLatencyMs   int64  `json:"median_latency_ms"`
	TargetLatencyMs   int64  `json:"target_latency_ms"`
	SubmittedAtHeight int64  `json:"submitted_at_height"`

	// ReferralVerified24h is the number of distinct publishers the verifier referred
	// (via publisher registration `referrer`) that were verified within the last 24 hours.
	// This field is intended to be computed off-chain when preparing settlement inputs.
	ReferralVerified24h int `json:"referral_verified_24h"`

	// WrongVerifications is the number of verification results that disagreed with the
	// majority verdict for the assigned publishers.
	WrongVerifications int `json:"wrong_verifications"`
}

// Validate ensures the report contains enough data to score contributions.
func (vr VerifierReport) Validate() error {
	if vr.Worker == "" {
		return fmt.Errorf("verifier worker address required")
	}
	if vr.Checks <= 0 {
		return fmt.Errorf("checks must be positive")
	}
	if vr.Matches < 0 || vr.Matches > vr.Checks {
		return fmt.Errorf("matches must lie within [0,checks]")
	}
	if vr.ExpectedChecks < 0 {
		return fmt.Errorf("expected checks must be >= 0")
	}
	if vr.TargetLatencyMs <= 0 {
		return fmt.Errorf("target latency must be positive")
	}
	if vr.ReferralVerified24h < 0 {
		return fmt.Errorf("referral verified 24h must be >= 0")
	}
	if vr.WrongVerifications < 0 {
		return fmt.Errorf("wrong verifications must be >= 0")
	}
	if vr.WrongVerifications > vr.Checks {
		return fmt.Errorf("wrong verifications cannot exceed checks")
	}
	return nil
}

// Score converts the report into a [0,1] weight using KPI weights.
func (vr VerifierReport) Score(weights VerifierRewardWeights, currentHeight, ttl int64) (sdkmath.LegacyDec, error) {
	if err := weights.Validate(); err != nil {
		return sdkmath.LegacyDec{}, err
	}
	if err := vr.Validate(); err != nil {
		return sdkmath.LegacyDec{}, err
	}
	if ttl <= 0 {
		return sdkmath.LegacyDec{}, fmt.Errorf("ttl must be positive")
	}
	if currentHeight <= 0 {
		return sdkmath.LegacyDec{}, fmt.Errorf("current height must be positive")
	}

	accuracy := ratioDec(vr.Matches, vr.Checks)
	expected := maxInt(vr.ExpectedChecks, 1)
	coverage := ratioDec(minInt(vr.Checks, expected), expected)

	latency := sdkmath.LegacyNewDec(vr.TargetLatencyMs).QuoInt64(maxInt64(vr.MedianLatencyMs, 1))
	if latency.GT(sdkmath.LegacyOneDec()) {
		latency = sdkmath.LegacyOneDec()
	}

	freshness := sdkmath.LegacyZeroDec()
	if vr.SubmittedAtHeight <= 0 || currentHeight <= vr.SubmittedAtHeight {
		freshness = sdkmath.LegacyOneDec()
	} else {
		delta := currentHeight - vr.SubmittedAtHeight
		if delta >= ttl {
			freshness = sdkmath.LegacyZeroDec()
		} else {
			freshness = sdkmath.LegacyOneDec().Sub(sdkmath.LegacyNewDec(delta).QuoInt64(ttl))
		}
	}

	score := sdkmath.LegacyZeroDec()
	score = score.Add(weights.Accuracy.Mul(accuracy))
	score = score.Add(weights.Coverage.Mul(coverage))
	score = score.Add(weights.Latency.Mul(latency))
	score = score.Add(weights.Freshness.Mul(freshness))

	// Penalize verifiers that frequently disagree with the majority verdict.
	// penaltyFactor = max(0, 1 - wrong/checks)
	if vr.Checks > 0 && vr.WrongVerifications > 0 {
		penalty := sdkmath.LegacyOneDec().Sub(ratioDec(vr.WrongVerifications, vr.Checks))
		if penalty.IsNegative() {
			penalty = sdkmath.LegacyZeroDec()
		}
		score = score.Mul(penalty)
	}

	return score, nil
}

// PublisherRewardOutcome encapsulates the results of a settlement cycle.
type PublisherRewardOutcome struct {
	PublisherAmount sdkmath.LegacyDec            `json:"publisher_amount"`
	VerifierAmounts map[string]sdkmath.LegacyDec `json:"verifier_amounts"`
	ProtocolAmount  sdkmath.LegacyDec            `json:"protocol_amount"`
	RolloverAmount  sdkmath.LegacyDec            `json:"rollover_amount"`
}

// SplitPublisherRewards divides the reward bucket among publisher, verifiers, and protocol sinks.
func SplitPublisherRewards(total sdkmath.LegacyDec, snapshot PublisherSnapshot, reports []VerifierReport, params PublisherParams) (PublisherRewardOutcome, error) {
	out := PublisherRewardOutcome{
		PublisherAmount: sdkmath.LegacyZeroDec(),
		VerifierAmounts: make(map[string]sdkmath.LegacyDec),
		ProtocolAmount:  sdkmath.LegacyZeroDec(),
		RolloverAmount:  sdkmath.LegacyZeroDec(),
	}
	if total.IsNegative() {
		return out, fmt.Errorf("total reward must be non-negative")
	}
	if err := params.Validate(); err != nil {
		return out, err
	}
	if total.IsZero() {
		return out, nil
	}

	links := snapshot.VerifiedCongridLinks
	if links < 0 {
		links = 0
	}
	factor := mustNewDec("0.10").Mul(sdkmath.LegacyNewDec(int64(links)))
	if factor.GT(sdkmath.LegacyOneDec()) {
		factor = sdkmath.LegacyOneDec()
	}
	eligibleTotal := total.Mul(factor)
	out.RolloverAmount = total.Sub(eligibleTotal)
	if eligibleTotal.IsZero() {
		return out, nil
	}

	publisherAmount := eligibleTotal.Mul(params.RewardSplit.PublisherShare)
	verifierPool := eligibleTotal.Mul(params.RewardSplit.VerifierShare)
	protocolAmount := eligibleTotal.Mul(params.RewardSplit.ProtocolShare)

	out.PublisherAmount = publisherAmount
	out.ProtocolAmount = protocolAmount

	if !verifierPool.IsZero() {
		if len(reports) == 0 {
			out.RolloverAmount = out.RolloverAmount.Add(verifierPool)
		} else {
			weights := make([]sdkmath.LegacyDec, len(reports))
			totalWeight := sdkmath.LegacyZeroDec()
			for i, report := range reports {
				score, err := report.Score(params.VerifierWeights, snapshot.CurrentHeight, params.VerificationTTL)
				if err != nil {
					return out, err
				}
				if score.LT(params.MinVerifierScore) {
					weights[i] = sdkmath.LegacyZeroDec()
					continue
				}

				// Verifier referral incentive:
				// Base payout factor is 10%. Each additional referred-and-verified publisher (within 24h)
				// increases it by 10%, capped at 100%.
				multiplier := mustNewDec("0.10").Mul(sdkmath.LegacyNewDec(int64(report.ReferralVerified24h + 1)))
				if multiplier.GT(sdkmath.LegacyOneDec()) {
					multiplier = sdkmath.LegacyOneDec()
				}

				effective := score.Mul(multiplier)
				weights[i] = effective
				totalWeight = totalWeight.Add(effective)
			}

			if totalWeight.IsZero() {
				out.RolloverAmount = out.RolloverAmount.Add(verifierPool)
			} else {
				allocated := sdkmath.LegacyZeroDec()
				for i, report := range reports {
					weight := weights[i]
					if weight.IsZero() {
						continue
					}
					portion := verifierPool.Mul(weight).Quo(totalWeight)
					allocated = allocated.Add(portion)
					existing, ok := out.VerifierAmounts[report.Worker]
					if !ok {
						existing = sdkmath.LegacyZeroDec()
					}
					out.VerifierAmounts[report.Worker] = existing.Add(portion)
				}

				if allocated.LT(verifierPool) {
					out.RolloverAmount = out.RolloverAmount.Add(verifierPool.Sub(allocated))
				}
			}
		}
	}

	consumedShares := publisherAmount.Add(verifierPool).Add(protocolAmount)
	if consumedShares.LT(eligibleTotal) {
		out.RolloverAmount = out.RolloverAmount.Add(eligibleTotal.Sub(consumedShares))
	}

	return out, nil
}

func ratioDec(numerator, denominator int) sdkmath.LegacyDec {
	if denominator <= 0 || numerator <= 0 {
		return sdkmath.LegacyZeroDec()
	}
	num := sdkmath.LegacyNewDec(int64(numerator))
	den := sdkmath.LegacyNewDec(int64(denominator))
	val := num.Quo(den)
	if val.GT(sdkmath.LegacyOneDec()) {
		return sdkmath.LegacyOneDec()
	}
	return val
}

func ensureSharesSumToOne(shares []sdkmath.LegacyDec, label string) error {
	total := sdkmath.LegacyZeroDec()
	for _, share := range shares {
		if share.IsNegative() {
			return fmt.Errorf("%s must be non-negative", label)
		}
		if share.GT(sdkmath.LegacyOneDec()) {
			return fmt.Errorf("%s component must be <= 1", label)
		}
		total = total.Add(share)
	}
	if !total.Equal(sdkmath.LegacyOneDec()) {
		return fmt.Errorf("%s must sum to 1, got %s", label, total)
	}
	return nil
}

func mustNewDec(val string) sdkmath.LegacyDec {
	return sdkmath.LegacyMustNewDecFromStr(val)
}

func maxInt(value, floor int) int {
	if value < floor {
		return floor
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt64(value, floor int64) int64 {
	if value < floor {
		return floor
	}
	return value
}

func ensureUnitInterval(dec sdkmath.LegacyDec, label string) error {
	if dec.IsNegative() {
		return fmt.Errorf("%s must be >= 0", label)
	}
	if dec.GT(sdkmath.LegacyOneDec()) {
		return fmt.Errorf("%s must be <= 1", label)
	}
	return nil
}
