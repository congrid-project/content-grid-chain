package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
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
		Service:                 "verifierd",
		Status:                  status,
		Reasons:                 reasons,
		StartedAt:               formatHealthTime(h.startedAt),
		UptimeSeconds:           int64(now.Sub(h.startedAt).Seconds()),
		CheckedAt:               formatHealthTime(now),
		StaleAfterSeconds:       int64(staleAfter.Seconds()),
		GRPCAddr:                cfg.GRPCAddr,
		VerifierAddress:         cfg.VerifierAddress,
		IndexerdBaseURL:         cfg.IndexerdBaseURL,
		PollIntervalSeconds:     cfg.PollIntervalSec,
		LastPollAttemptAt:       formatHealthTime(h.lastPollAttemptAt),
		LastPollSuccessAt:       formatHealthTime(h.lastPollSuccessAt),
		LastPollError:           h.lastPollError,
		ConsecutiveErrors:       h.consecutiveErrors,
		LastScanAssignments:     h.lastAssignmentScan,
		InFlightAssignments:     inFlight,
		PendingReveals:          pending,
		LastAssignmentStartedAt: formatHealthTime(h.lastAssignmentStartedAt),
		LastAssignmentEndedAt:   formatHealthTime(h.lastAssignmentEndedAt),
		LastAssignmentKey:       h.lastAssignmentKey,
		LastAssignmentError:     h.lastAssignmentError,
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
