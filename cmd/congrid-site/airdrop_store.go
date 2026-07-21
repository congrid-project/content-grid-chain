package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"

	registry "content-grid-chain/x/registry"
)

type claimStatus string

const (
	claimStatusVerified       claimStatus = "verified"
	claimStatusSubmitting     claimStatus = "submitting"
	claimStatusBroadcast      claimStatus = "broadcast"
	claimStatusConfirmed      claimStatus = "confirmed"
	claimStatusFailed         claimStatus = "failed"
	claimStatusNeedsReconcile claimStatus = "needs_reconcile"
)

type claimInfo struct {
	PrimaryDomain   string      `json:"primary_domain"`
	Domain          string      `json:"domain"`
	Wallet          string      `json:"wallet"`
	Amount          string      `json:"amount,omitempty"`
	Denom           string      `json:"denom,omitempty"`
	Status          claimStatus `json:"status,omitempty"`
	TxHash          string      `json:"tx_hash"`
	ClaimedAt       time.Time   `json:"claimed_at"`
	UpdatedAt       time.Time   `json:"updated_at,omitempty"`
	BroadcastAt     time.Time   `json:"broadcast_at,omitempty"`
	ConfirmedAt     time.Time   `json:"confirmed_at,omitempty"`
	SubmissionCount int         `json:"submission_count,omitempty"`
	LastError       string      `json:"last_error,omitempty"`
}

type claimStore interface {
	Reserve(context.Context, claimInfo) (claimInfo, bool, error)
	Get(context.Context, string) (claimInfo, bool, error)
	Next(context.Context, claimStatus) (claimInfo, bool, error)
	MarkSubmitting(context.Context, string) (bool, error)
	MarkBroadcast(context.Context, string, string, time.Time) error
	MarkConfirmed(context.Context, string, time.Time) error
	MarkDeliveryFailed(context.Context, string, string) error
	MarkConfirmationUncertain(context.Context, string, string) error
	MarkTerminal(context.Context, string, claimStatus, string) error
	RecoverInterrupted(context.Context) (int64, error)
	Close() error
}

type sqlClaimStore struct {
	db      *sql.DB
	dialect string
}

func openSQLClaimStore(ctx context.Context, driver, dsn, sqlitePath string) (*sqlClaimStore, error) {
	driver = normalizeDBDriver(driver)
	if driver == "" {
		return nil, fmt.Errorf("airdrop db driver required")
	}

	var legacy map[string]claimInfo
	var backupPath string
	var err error
	if driver == "sqlite" && strings.TrimSpace(dsn) == "" {
		sqlitePath = strings.TrimSpace(sqlitePath)
		if sqlitePath == "" {
			return nil, fmt.Errorf("airdrop sqlite path required")
		}
		legacy, backupPath, err = moveLegacyJSONAside(sqlitePath)
		if err != nil {
			return nil, err
		}
		dsn = sqlitePath
	}
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("airdrop db dsn required for driver %s", driver)
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, restoreLegacyAfterOpenFailure(sqlitePath, backupPath, err)
	}
	store := &sqlClaimStore{db: db, dialect: driver}
	if driver == "sqlite" {
		// A single connection avoids connection-local PRAGMA differences and also
		// serializes SQLite writes inside this process.
		db.SetMaxOpenConns(1)
		for _, pragma := range []string{
			"PRAGMA journal_mode = WAL",
			"PRAGMA busy_timeout = 5000",
			"PRAGMA foreign_keys = ON",
		} {
			if _, err := db.ExecContext(ctx, pragma); err != nil {
				_ = db.Close()
				return nil, restoreLegacyAfterOpenFailure(sqlitePath, backupPath, fmt.Errorf("configure sqlite: %w", err))
			}
		}
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, restoreLegacyAfterOpenFailure(sqlitePath, backupPath, fmt.Errorf("connect to airdrop db: %w", err))
	}
	if err := store.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, restoreLegacyAfterOpenFailure(sqlitePath, backupPath, err)
	}
	if len(legacy) > 0 {
		if err := store.importLegacy(ctx, legacy); err != nil {
			_ = db.Close()
			return nil, restoreLegacyAfterOpenFailure(sqlitePath, backupPath, fmt.Errorf("import legacy airdrop claims: %w", err))
		}
	}
	return store, nil
}

func normalizeDBDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "sqlite", "sqlite3":
		return "sqlite"
	case "postgres", "postgresql", "pq":
		return "postgres"
	default:
		return strings.ToLower(strings.TrimSpace(driver))
	}
}

func (s *sqlClaimStore) migrate(ctx context.Context) error {
	const schema = `
CREATE TABLE IF NOT EXISTS airdrop_claims (
    site_key TEXT PRIMARY KEY,
    requested_domain TEXT NOT NULL,
    wallet TEXT NOT NULL,
    amount TEXT NOT NULL,
    denom TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('verified', 'submitting', 'broadcast', 'confirmed', 'failed', 'needs_reconcile')),
    tx_hash TEXT,
    claimed_at_unix BIGINT NOT NULL,
    updated_at_unix BIGINT NOT NULL,
    broadcast_at_unix BIGINT NOT NULL DEFAULT 0,
    confirmed_at_unix BIGINT NOT NULL DEFAULT 0,
    submission_count BIGINT NOT NULL DEFAULT 0,
    last_error TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS airdrop_claims_tx_hash_uq
    ON airdrop_claims(tx_hash)
    WHERE tx_hash IS NOT NULL AND tx_hash <> '';
CREATE INDEX IF NOT EXISTS airdrop_claims_status_created_idx
    ON airdrop_claims(status, claimed_at_unix);
`
	if _, err := s.db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("migrate airdrop db: %w", err)
	}
	return nil
}

func (s *sqlClaimStore) importLegacy(ctx context.Context, claims map[string]claimInfo) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	for key, claim := range claims {
		claim.PrimaryDomain = strings.TrimSpace(strings.ToLower(firstNonEmptyString(claim.PrimaryDomain, key)))
		claim.Domain = strings.TrimSpace(strings.ToLower(firstNonEmptyString(claim.Domain, claim.PrimaryDomain)))
		if claim.PrimaryDomain == "" || claim.Domain == "" || strings.TrimSpace(claim.Wallet) == "" {
			return fmt.Errorf("invalid legacy claim %q", key)
		}
		expectedPrimary, err := registry.GetPrimaryDomain(claim.Domain)
		if err != nil || expectedPrimary != claim.PrimaryDomain {
			return fmt.Errorf("legacy claim %q has inconsistent domain key", key)
		}
		claim.Status = claimStatusNeedsReconcile
		if strings.TrimSpace(claim.TxHash) != "" {
			claim.Status = claimStatusConfirmed
		}
		if claim.ClaimedAt.IsZero() {
			claim.ClaimedAt = now
		}
		claim.UpdatedAt = now
		if claim.Amount == "" {
			claim.Amount = "unknown"
		}
		if claim.Denom == "" {
			claim.Denom = "unknown"
		}
		query := s.bind(`INSERT INTO airdrop_claims (
site_key, requested_domain, wallet, amount, denom, status, tx_hash,
claimed_at_unix, updated_at_unix, broadcast_at_unix, confirmed_at_unix,
submission_count, last_error
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (site_key) DO NOTHING`)
		if _, err := tx.ExecContext(ctx, query,
			claim.PrimaryDomain, claim.Domain, strings.TrimSpace(claim.Wallet), claim.Amount, claim.Denom,
			string(claim.Status), nullableString(claim.TxHash), claim.ClaimedAt.Unix(), claim.UpdatedAt.Unix(),
			unixOrZero(claim.BroadcastAt), unixOrZero(claim.ConfirmedAt), claim.SubmissionCount, claim.LastError,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sqlClaimStore) Reserve(ctx context.Context, claim claimInfo) (claimInfo, bool, error) {
	claim.PrimaryDomain = strings.TrimSpace(strings.ToLower(claim.PrimaryDomain))
	claim.Domain = strings.TrimSpace(strings.ToLower(claim.Domain))
	claim.Wallet = strings.TrimSpace(claim.Wallet)
	expectedPrimary, err := registry.GetPrimaryDomain(claim.Domain)
	if err != nil || expectedPrimary != claim.PrimaryDomain {
		return claimInfo{}, false, fmt.Errorf("airdrop claim domain does not match its simplified primary key")
	}
	if claim.Wallet == "" || claim.Amount == "" || claim.Denom == "" {
		return claimInfo{}, false, fmt.Errorf("airdrop claim wallet, amount, and denom are required")
	}
	now := time.Now().UTC()
	claim.Status = claimStatusVerified
	claim.ClaimedAt = now
	claim.UpdatedAt = now

	query := s.bind(`INSERT INTO airdrop_claims (
site_key, requested_domain, wallet, amount, denom, status, tx_hash,
claimed_at_unix, updated_at_unix, broadcast_at_unix, confirmed_at_unix,
submission_count, last_error
) VALUES (?, ?, ?, ?, ?, ?, NULL, ?, ?, 0, 0, 0, '')
ON CONFLICT (site_key) DO NOTHING`)
	result, err := s.db.ExecContext(ctx, query,
		claim.PrimaryDomain, claim.Domain, claim.Wallet, claim.Amount, claim.Denom,
		string(claim.Status), claim.ClaimedAt.Unix(), claim.UpdatedAt.Unix(),
	)
	if err != nil {
		return claimInfo{}, false, fmt.Errorf("reserve airdrop claim: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return claimInfo{}, false, fmt.Errorf("inspect airdrop reservation: %w", err)
	}
	if rows == 1 {
		return claim, true, nil
	}
	existing, ok, err := s.Get(ctx, claim.PrimaryDomain)
	if err != nil {
		return claimInfo{}, false, err
	}
	if !ok {
		return claimInfo{}, false, fmt.Errorf("airdrop reservation conflict without existing row")
	}
	return existing, false, nil
}

func (s *sqlClaimStore) Get(ctx context.Context, siteKey string) (claimInfo, bool, error) {
	query := s.bind(claimSelect + ` WHERE site_key = ?`)
	claim, err := scanClaim(s.db.QueryRowContext(ctx, query, strings.TrimSpace(strings.ToLower(siteKey))))
	if errors.Is(err, sql.ErrNoRows) {
		return claimInfo{}, false, nil
	}
	if err != nil {
		return claimInfo{}, false, fmt.Errorf("get airdrop claim: %w", err)
	}
	return claim, true, nil
}

func (s *sqlClaimStore) Next(ctx context.Context, status claimStatus) (claimInfo, bool, error) {
	query := s.bind(claimSelect + ` WHERE status = ? ORDER BY claimed_at_unix ASC LIMIT 1`)
	claim, err := scanClaim(s.db.QueryRowContext(ctx, query, string(status)))
	if errors.Is(err, sql.ErrNoRows) {
		return claimInfo{}, false, nil
	}
	if err != nil {
		return claimInfo{}, false, fmt.Errorf("get next %s airdrop claim: %w", status, err)
	}
	return claim, true, nil
}

func (s *sqlClaimStore) MarkSubmitting(ctx context.Context, siteKey string) (bool, error) {
	query := s.bind(`UPDATE airdrop_claims
SET status = ?, updated_at_unix = ?, submission_count = submission_count + 1, last_error = ''
WHERE site_key = ? AND status = ?`)
	result, err := s.db.ExecContext(ctx, query,
		string(claimStatusSubmitting), time.Now().UTC().Unix(), siteKey, string(claimStatusVerified),
	)
	if err != nil {
		return false, fmt.Errorf("mark airdrop submitting: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

func (s *sqlClaimStore) MarkBroadcast(ctx context.Context, siteKey, txHash string, at time.Time) error {
	return s.transition(ctx, siteKey, claimStatusSubmitting, claimStatusBroadcast, strings.TrimSpace(txHash), at, time.Time{}, "")
}

func (s *sqlClaimStore) MarkConfirmed(ctx context.Context, siteKey string, at time.Time) error {
	return s.transition(ctx, siteKey, claimStatusBroadcast, claimStatusConfirmed, "", time.Time{}, at, "")
}

func (s *sqlClaimStore) MarkDeliveryFailed(ctx context.Context, siteKey, message string) error {
	return s.transition(ctx, siteKey, claimStatusBroadcast, claimStatusFailed, "", time.Time{}, time.Time{}, message)
}

func (s *sqlClaimStore) MarkConfirmationUncertain(ctx context.Context, siteKey, message string) error {
	return s.transition(ctx, siteKey, claimStatusBroadcast, claimStatusNeedsReconcile, "", time.Time{}, time.Time{}, message)
}

func (s *sqlClaimStore) MarkTerminal(ctx context.Context, siteKey string, status claimStatus, message string) error {
	if status != claimStatusFailed && status != claimStatusNeedsReconcile {
		return fmt.Errorf("invalid terminal airdrop status %q", status)
	}
	query := s.bind(`UPDATE airdrop_claims
SET status = ?, updated_at_unix = ?, last_error = ?
WHERE site_key = ? AND status = ?`)
	result, err := s.db.ExecContext(ctx, query,
		string(status), time.Now().UTC().Unix(), truncateError(message), siteKey, string(claimStatusSubmitting),
	)
	if err != nil {
		return fmt.Errorf("mark airdrop %s: %w", status, err)
	}
	return requireOneRow(result, "terminal airdrop transition")
}

func (s *sqlClaimStore) RecoverInterrupted(ctx context.Context) (int64, error) {
	query := s.bind(`UPDATE airdrop_claims
SET status = ?, updated_at_unix = ?, last_error = ?
WHERE status = ?`)
	result, err := s.db.ExecContext(ctx, query,
		string(claimStatusNeedsReconcile), time.Now().UTC().Unix(),
		"server stopped while transaction submission was in progress; operator reconciliation required",
		string(claimStatusSubmitting),
	)
	if err != nil {
		return 0, fmt.Errorf("recover interrupted airdrop claims: %w", err)
	}
	return result.RowsAffected()
}

func (s *sqlClaimStore) transition(
	ctx context.Context,
	siteKey string,
	from, to claimStatus,
	txHash string,
	broadcastAt, confirmedAt time.Time,
	lastError string,
) error {
	query := s.bind(`UPDATE airdrop_claims
SET status = ?, tx_hash = CASE WHEN ? <> '' THEN ? ELSE tx_hash END,
    updated_at_unix = ?,
    broadcast_at_unix = CASE WHEN ? > 0 THEN ? ELSE broadcast_at_unix END,
    confirmed_at_unix = CASE WHEN ? > 0 THEN ? ELSE confirmed_at_unix END,
    last_error = ?
WHERE site_key = ? AND status = ?`)
	result, err := s.db.ExecContext(ctx, query,
		string(to), txHash, txHash, time.Now().UTC().Unix(),
		unixOrZero(broadcastAt), unixOrZero(broadcastAt),
		unixOrZero(confirmedAt), unixOrZero(confirmedAt),
		truncateError(lastError), siteKey, string(from),
	)
	if err != nil {
		return fmt.Errorf("transition airdrop claim %s to %s: %w", siteKey, to, err)
	}
	return requireOneRow(result, "airdrop transition")
}

func (s *sqlClaimStore) Close() error { return s.db.Close() }

const claimSelect = `SELECT site_key, requested_domain, wallet, amount, denom, status,
COALESCE(tx_hash, ''), claimed_at_unix, updated_at_unix, broadcast_at_unix,
confirmed_at_unix, submission_count, last_error
FROM airdrop_claims`

type rowScanner interface {
	Scan(...any) error
}

func scanClaim(row rowScanner) (claimInfo, error) {
	var claim claimInfo
	var status string
	var claimedAt, updatedAt, broadcastAt, confirmedAt int64
	err := row.Scan(
		&claim.PrimaryDomain, &claim.Domain, &claim.Wallet, &claim.Amount, &claim.Denom,
		&status, &claim.TxHash, &claimedAt, &updatedAt, &broadcastAt, &confirmedAt,
		&claim.SubmissionCount, &claim.LastError,
	)
	if err != nil {
		return claimInfo{}, err
	}
	claim.Status = claimStatus(status)
	claim.ClaimedAt = time.Unix(claimedAt, 0).UTC()
	claim.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	claim.BroadcastAt = timeFromUnix(broadcastAt)
	claim.ConfirmedAt = timeFromUnix(confirmedAt)
	return claim, nil
}

func (s *sqlClaimStore) bind(query string) string {
	if s.dialect != "postgres" {
		return query
	}
	var b strings.Builder
	index := 1
	for _, r := range query {
		if r == '?' {
			fmt.Fprintf(&b, "$%d", index)
			index++
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func moveLegacyJSONAside(path string) (map[string]claimInfo, string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, "", fmt.Errorf("create airdrop db directory: %w", err)
		}
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("inspect airdrop db: %w", err)
	}
	trimmed := strings.TrimSpace(string(b))
	if strings.HasPrefix(trimmed, "SQLite format 3") {
		return nil, "", nil
	}
	if !strings.HasPrefix(trimmed, "{") {
		return nil, "", fmt.Errorf("airdrop db %s is neither SQLite nor the legacy JSON format", path)
	}
	var claims map[string]claimInfo
	if err := json.Unmarshal(b, &claims); err != nil {
		return nil, "", fmt.Errorf("parse legacy airdrop db %s: %w", path, err)
	}
	backup := path + ".json.bak"
	if _, err := os.Stat(backup); err == nil {
		return nil, "", fmt.Errorf("legacy airdrop backup already exists: %s", backup)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, "", fmt.Errorf("inspect legacy airdrop backup: %w", err)
	}
	if err := os.Rename(path, backup); err != nil {
		return nil, "", fmt.Errorf("back up legacy airdrop db: %w", err)
	}
	return claims, backup, nil
}

func restoreLegacyAfterOpenFailure(path, backup string, cause error) error {
	if backup == "" {
		return cause
	}
	_ = os.Remove(path)
	_ = os.Remove(path + "-wal")
	_ = os.Remove(path + "-shm")
	if err := os.Rename(backup, path); err != nil {
		return fmt.Errorf("%w; additionally failed to restore legacy db from %s: %v", cause, backup, err)
	}
	return cause
}

func requireOneRow(result sql.Result, operation string) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("%s affected %d rows", operation, rows)
	}
	return nil
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UTC().Unix()
}

func timeFromUnix(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.Unix(value, 0).UTC()
}

func truncateError(message string) string {
	message = strings.TrimSpace(message)
	if len(message) > 2_000 {
		return message[:2_000]
	}
	return message
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
