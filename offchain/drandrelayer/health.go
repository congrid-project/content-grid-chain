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

	lastSyncAttemptAt time.Time
	lastSyncSuccessAt time.Time
	lastSyncError     string
	consecutiveErrors int

	onChainRound       uint64
	latestDrandRound   uint64
	lastSubmittedRound uint64
	lastSubmittedAt    time.Time
	nextSubmitTryAt    time.Time
	lastSkippedRound   uint64
	throttledUntil     time.Time
}

type relayerHealthSnapshot struct {
	Service             string   `json:"service"`
	Status              string   `json:"status"`
	Reasons             []string `json:"reasons,omitempty"`
	StartedAt           string   `json:"started_at"`
	UptimeSeconds       int64    `json:"uptime_seconds"`
	CheckedAt           string   `json:"checked_at"`
	StaleAfterSeconds   int64    `json:"stale_after_seconds"`
	GRPCAddr            string   `json:"grpc_addr"`
	DrandAPIBaseURL     string   `json:"drand_api_base_url"`
	DrandChainHash      string   `json:"drand_chain_hash"`
	PollIntervalSeconds int      `json:"poll_interval_seconds"`

	LastSyncAttemptAt string `json:"last_sync_attempt_at,omitempty"`
	LastSyncSuccessAt string `json:"last_sync_success_at,omitempty"`
	LastSyncError     string `json:"last_sync_error,omitempty"`
	ConsecutiveErrors int    `json:"consecutive_errors"`

	OnChainRound       uint64 `json:"on_chain_round"`
	LatestDrandRound   uint64 `json:"latest_drand_round"`
	LastSubmittedRound uint64 `json:"last_submitted_round,omitempty"`
	LastSubmittedAt    string `json:"last_submitted_at,omitempty"`
	NextSubmitTryAt    string `json:"next_submit_try_at,omitempty"`
	LastSkippedRound   uint64 `json:"last_skipped_round,omitempty"`
	ThrottledUntil     string `json:"throttled_until,omitempty"`
}

func newDaemonHealth(pollInterval time.Duration) *daemonHealth {
	return &daemonHealth{
		startedAt:    time.Now().UTC(),
		pollInterval: pollInterval,
	}
}

func (h *daemonHealth) recordSyncAttempt() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastSyncAttemptAt = time.Now().UTC()
}

func (h *daemonHealth) recordSyncResult(err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if err != nil {
		h.lastSyncError = err.Error()
		h.consecutiveErrors++
		return
	}
	h.lastSyncSuccessAt = time.Now().UTC()
	h.lastSyncError = ""
	h.consecutiveErrors = 0
}

func (h *daemonHealth) recordOnChainRound(round uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onChainRound = round
}

func (h *daemonHealth) recordLatestDrandRound(round uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.latestDrandRound = round
}

func (h *daemonHealth) recordSubmission(round uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastSubmittedRound = round
	h.lastSubmittedAt = time.Now().UTC()
}

func (h *daemonHealth) recordNextSubmitTryAt(t time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextSubmitTryAt = t.UTC()
}

func (h *daemonHealth) recordThrottle(round uint64, until time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastSkippedRound = round
	h.throttledUntil = until.UTC()
}

func (h *daemonHealth) snapshot(cfg Config) (relayerHealthSnapshot, int) {
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
	return relayerHealthSnapshot{
		Service:             "drand-relayer",
		Status:              status,
		Reasons:             reasons,
		StartedAt:           formatHealthTime(h.startedAt),
		UptimeSeconds:       int64(now.Sub(h.startedAt).Seconds()),
		CheckedAt:           formatHealthTime(now),
		StaleAfterSeconds:   int64(staleAfter.Seconds()),
		GRPCAddr:            cfg.GRPCAddr,
		DrandAPIBaseURL:     cfg.DrandAPIBaseURL,
		DrandChainHash:      cfg.DrandChainHash,
		PollIntervalSeconds: cfg.PollIntervalSec,
		LastSyncAttemptAt:   formatHealthTime(h.lastSyncAttemptAt),
		LastSyncSuccessAt:   formatHealthTime(h.lastSyncSuccessAt),
		LastSyncError:       h.lastSyncError,
		ConsecutiveErrors:   h.consecutiveErrors,
		OnChainRound:        h.onChainRound,
		LatestDrandRound:    h.latestDrandRound,
		LastSubmittedRound:  h.lastSubmittedRound,
		LastSubmittedAt:     formatHealthTime(h.lastSubmittedAt),
		NextSubmitTryAt:     formatHealthTime(h.nextSubmitTryAt),
		LastSkippedRound:    h.lastSkippedRound,
		ThrottledUntil:      formatHealthTime(h.throttledUntil),
	}, httpStatus
}

func (h *daemonHealth) readinessReasons(now time.Time, staleAfter time.Duration) []string {
	var reasons []string
	if h.lastSyncSuccessAt.IsZero() {
		reasons = append(reasons, "no successful sync yet")
	}
	if h.consecutiveErrors > 0 {
		msg := "last sync failed"
		if strings.TrimSpace(h.lastSyncError) != "" {
			msg += ": " + h.lastSyncError
		}
		reasons = append(reasons, msg)
	}
	if !h.lastSyncSuccessAt.IsZero() && now.Sub(h.lastSyncSuccessAt) > staleAfter {
		reasons = append(reasons, "last successful sync is stale")
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

func healthRoutes(cfg Config, health *daemonHealth) http.Handler {
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
		snapshot, status := health.snapshot(cfg)
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
