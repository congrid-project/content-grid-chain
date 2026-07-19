package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	registrypb "content-grid-chain/x/registry/typespb"
)

type daemonHealth struct {
	mu           sync.Mutex
	startedAt    time.Time
	pollInterval time.Duration

	lastPollAttemptAt  time.Time
	lastPollSuccessAt  time.Time
	lastPollError      string
	consecutiveErrors  int
	lastAssignmentScan int

	lastAssignmentStartedAt time.Time
	lastAssignmentEndedAt   time.Time
	lastAssignmentKey       string
	lastAssignmentError     string

	drandEnabled             bool
	drandPending             bool
	drandSubmitted           bool
	drandChainHash           string
	drandRoundStartUnix      int64
	drandRequiredRound       uint64
	drandRequiredBeaconUnix  int64
	drandRelayRank           int
	drandRelayTotal          int
	drandRelayNotBeforeUnix  int64
	lastDrandSubmissionAt    time.Time
	lastDrandSubmissionRound uint64
	lastDrandError           string
}

type verifierHealthSnapshot struct {
	Service             string   `json:"service"`
	Status              string   `json:"status"`
	Reasons             []string `json:"reasons,omitempty"`
	StartedAt           string   `json:"started_at"`
	UptimeSeconds       int64    `json:"uptime_seconds"`
	CheckedAt           string   `json:"checked_at"`
	StaleAfterSeconds   int64    `json:"stale_after_seconds"`
	GRPCAddr            string   `json:"grpc_addr"`
	VerifierAddress     string   `json:"verifier_address"`
	IndexerdBaseURL     string   `json:"indexerd_base_url,omitempty"`
	PollIntervalSeconds int      `json:"poll_interval_seconds"`

	LastPollAttemptAt   string `json:"last_poll_attempt_at,omitempty"`
	LastPollSuccessAt   string `json:"last_poll_success_at,omitempty"`
	LastPollError       string `json:"last_poll_error,omitempty"`
	ConsecutiveErrors   int    `json:"consecutive_errors"`
	LastScanAssignments int    `json:"last_scan_assignments"`

	InFlightAssignments int    `json:"in_flight_assignments"`
	PendingReveals      int    `json:"pending_reveals"`
	PendingRevealError  string `json:"pending_reveal_error,omitempty"`

	LastAssignmentStartedAt string `json:"last_assignment_started_at,omitempty"`
	LastAssignmentEndedAt   string `json:"last_assignment_ended_at,omitempty"`
	LastAssignmentKey       string `json:"last_assignment_key,omitempty"`
	LastAssignmentError     string `json:"last_assignment_error,omitempty"`

	DrandRelayDisabled       bool   `json:"drand_relay_disabled"`
	DrandEnabled             bool   `json:"drand_enabled"`
	DrandPending             bool   `json:"drand_pending"`
	DrandSubmitted           bool   `json:"drand_submitted"`
	DrandChainHash           string `json:"drand_chain_hash,omitempty"`
	DrandRoundStartUnix      int64  `json:"drand_round_start_unix,omitempty"`
	DrandRequiredRound       uint64 `json:"drand_required_round,omitempty"`
	DrandRequiredBeaconUnix  int64  `json:"drand_required_beacon_unix,omitempty"`
	DrandRelayRank           int    `json:"drand_relay_rank"`
	DrandRelayTotal          int    `json:"drand_relay_total,omitempty"`
	DrandRelayNotBeforeUnix  int64  `json:"drand_relay_not_before_unix,omitempty"`
	LastDrandSubmissionAt    string `json:"last_drand_submission_at,omitempty"`
	LastDrandSubmissionRound uint64 `json:"last_drand_submission_round,omitempty"`
	LastDrandError           string `json:"last_drand_error,omitempty"`
}

func newDaemonHealth(pollInterval time.Duration) *daemonHealth {
	return &daemonHealth{
		startedAt:    time.Now().UTC(),
		pollInterval: pollInterval,
	}
}

func (h *daemonHealth) recordPollAttempt() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastPollAttemptAt = time.Now().UTC()
}

func (h *daemonHealth) recordPollResult(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		h.lastPollError = err.Error()
		h.consecutiveErrors++
		return
	}
	h.lastPollSuccessAt = time.Now().UTC()
	h.lastPollError = ""
	h.consecutiveErrors = 0
}

func (h *daemonHealth) recordAssignmentScan(count int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastAssignmentScan = count
}

func (h *daemonHealth) recordAssignmentStarted(key string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now().UTC()
	h.lastAssignmentStartedAt = now
	h.lastAssignmentKey = key
	h.lastAssignmentError = ""
}

func (h *daemonHealth) recordAssignmentFinished(key string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastAssignmentEndedAt = time.Now().UTC()
	h.lastAssignmentKey = key
	if err != nil {
		h.lastAssignmentError = err.Error()
		return
	}
	h.lastAssignmentError = ""
}

func (h *daemonHealth) recordDrandRequirement(requirement *registrypb.QueryDrandRequirementResponse) {
	if requirement == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.drandEnabled = requirement.GetEnabled()
	h.drandPending = requirement.GetPending()
	h.drandSubmitted = requirement.GetSubmitted()
	h.drandChainHash = requirement.GetDrandChainHash()
	h.drandRoundStartUnix = requirement.GetRoundStartUnix()
	h.drandRequiredRound = requirement.GetRequiredDrandRound()
	h.drandRequiredBeaconUnix = requirement.GetRequiredBeaconUnix()
	if !h.drandPending || h.drandSubmitted {
		h.lastDrandError = ""
		h.drandRelayRank = 0
		h.drandRelayTotal = 0
		h.drandRelayNotBeforeUnix = 0
	}
}

func (h *daemonHealth) recordDrandRelaySchedule(rank, total int, notBeforeUnix int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.drandRelayRank = rank
	h.drandRelayTotal = total
	h.drandRelayNotBeforeUnix = notBeforeUnix
}

func (h *daemonHealth) recordDrandSubmission(round uint64, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastDrandSubmissionAt = time.Now().UTC()
	if round > 0 {
		h.lastDrandSubmissionRound = round
	}
	if err != nil {
		h.lastDrandError = err.Error()
		return
	}
	h.lastDrandError = ""
	if round > 0 && round == h.drandRequiredRound {
		h.drandSubmitted = true
	}
}

func (h *daemonHealth) snapshot(cfg Config, inFlight, pending int, pendingErr error) (verifierHealthSnapshot, int) {
	now := time.Now().UTC()
	h.mu.Lock()
	defer h.mu.Unlock()

	staleAfter := h.staleAfter()
	reasons := h.readinessReasons(now, staleAfter)
	status := "ready"
	httpStatus := http.StatusOK
	if len(reasons) > 0 {
		status = "not_ready"
		httpStatus = http.StatusServiceUnavailable
	}
	out := verifierHealthSnapshot{
		Service:                  "verifierd",
		Status:                   status,
		Reasons:                  reasons,
		StartedAt:                formatHealthTime(h.startedAt),
		UptimeSeconds:            int64(now.Sub(h.startedAt).Seconds()),
		CheckedAt:                formatHealthTime(now),
		StaleAfterSeconds:        int64(staleAfter.Seconds()),
		GRPCAddr:                 cfg.GRPCAddr,
		VerifierAddress:          cfg.VerifierAddress,
		IndexerdBaseURL:          cfg.IndexerdBaseURL,
		PollIntervalSeconds:      cfg.PollIntervalSec,
		LastPollAttemptAt:        formatHealthTime(h.lastPollAttemptAt),
		LastPollSuccessAt:        formatHealthTime(h.lastPollSuccessAt),
		LastPollError:            h.lastPollError,
		ConsecutiveErrors:        h.consecutiveErrors,
		LastScanAssignments:      h.lastAssignmentScan,
		InFlightAssignments:      inFlight,
		PendingReveals:           pending,
		LastAssignmentStartedAt:  formatHealthTime(h.lastAssignmentStartedAt),
		LastAssignmentEndedAt:    formatHealthTime(h.lastAssignmentEndedAt),
		LastAssignmentKey:        h.lastAssignmentKey,
		LastAssignmentError:      h.lastAssignmentError,
		DrandRelayDisabled:       cfg.Drand.Disabled,
		DrandEnabled:             h.drandEnabled,
		DrandPending:             h.drandPending,
		DrandSubmitted:           h.drandSubmitted,
		DrandChainHash:           h.drandChainHash,
		DrandRoundStartUnix:      h.drandRoundStartUnix,
		DrandRequiredRound:       h.drandRequiredRound,
		DrandRequiredBeaconUnix:  h.drandRequiredBeaconUnix,
		DrandRelayRank:           h.drandRelayRank,
		DrandRelayTotal:          h.drandRelayTotal,
		DrandRelayNotBeforeUnix:  h.drandRelayNotBeforeUnix,
		LastDrandSubmissionAt:    formatHealthTime(h.lastDrandSubmissionAt),
		LastDrandSubmissionRound: h.lastDrandSubmissionRound,
		LastDrandError:           h.lastDrandError,
	}
	if pendingErr != nil {
		out.PendingRevealError = pendingErr.Error()
	}
	return out, httpStatus
}

func (h *daemonHealth) readinessReasons(now time.Time, staleAfter time.Duration) []string {
	var reasons []string
	if h.lastPollSuccessAt.IsZero() {
		reasons = append(reasons, "no successful poll yet")
	}
	if h.consecutiveErrors > 0 {
		msg := "last poll failed"
		if strings.TrimSpace(h.lastPollError) != "" {
			msg += ": " + h.lastPollError
		}
		reasons = append(reasons, msg)
	}
	if !h.lastPollSuccessAt.IsZero() && now.Sub(h.lastPollSuccessAt) > staleAfter {
		reasons = append(reasons, "last successful poll is stale")
	}
	if h.lastDrandError != "" {
		reasons = append(reasons, "required drand delivery failed: "+h.lastDrandError)
	}
	return reasons
}

func (h *daemonHealth) staleAfter() time.Duration {
	staleAfter := h.pollInterval*3 + 30*time.Second
	if staleAfter < time.Minute {
		return time.Minute
	}
	return staleAfter
}

func healthRoutes(cfg Config, health *daemonHealth, agent *Agent) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		pending, pendingErr := agent.pendingRevealCount()
		snapshot, status := health.snapshot(cfg, agent.inFlightCount(), pending, pendingErr)
		writeHealthJSON(w, status, snapshot)
	})
	return mux
}

func writeHealthJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func formatHealthTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
