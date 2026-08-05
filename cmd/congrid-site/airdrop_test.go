package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLClaimStoreEnforcesWebsiteOnlyUniqueness(t *testing.T) {
	store := openTestClaimStore(t)
	ctx := context.Background()

	first, created, err := store.Reserve(ctx, testClaim("example.com", "congrid1wallet-a"))
	if err != nil || !created {
		t.Fatalf("reserve first claim: created=%t err=%v", created, err)
	}

	existing, created, err := store.Reserve(ctx, testClaim("example.com", "congrid1wallet-b"))
	if err != nil {
		t.Fatalf("reserve duplicate website: %v", err)
	}
	if created {
		t.Fatal("same website must not create a second claim")
	}
	if existing.Wallet != first.Wallet {
		t.Fatalf("duplicate changed wallet: got %q want %q", existing.Wallet, first.Wallet)
	}

	_, created, err = store.Reserve(ctx, testClaim("another.net", first.Wallet))
	if err != nil || !created {
		t.Fatalf("same wallet must be allowed for another website: created=%t err=%v", created, err)
	}
}

func TestValidateCongridAccountAddressUsesChainPrefix(t *testing.T) {
	const address = "congrid1fglanlkvqtyznlw3flu88680zmctyug8qr03pj"
	if err := validateCongridAccountAddress(address); err != nil {
		t.Fatalf("valid congrid address rejected: %v", err)
	}

	invalid := []string{
		"cosmos1fglanlkvqtyznlw3flu88680zmctyug8qr03pj",
		"congrid1not-a-valid-address",
		"",
	}
	for _, candidate := range invalid {
		if err := validateCongridAccountAddress(candidate); err == nil {
			t.Fatalf("invalid address accepted: %q", candidate)
		}
	}
}

func TestSQLClaimStoreRetainsLastTwoLabelWebsiteKey(t *testing.T) {
	store := openTestClaimStore(t)
	ctx := context.Background()

	_, created, err := store.Reserve(ctx, claimInfo{
		PrimaryDomain: "co.uk",
		Domain:        "www.example.co.uk",
		Wallet:        "congrid1wallet-a",
		Amount:        "25000",
		Denom:         "ucongrid",
	})
	if err != nil || !created {
		t.Fatalf("reserve co.uk claim: created=%t err=%v", created, err)
	}
	_, created, err = store.Reserve(ctx, claimInfo{
		PrimaryDomain: "co.uk",
		Domain:        "another.co.uk",
		Wallet:        "congrid1wallet-b",
		Amount:        "25000",
		Denom:         "ucongrid",
	})
	if err != nil {
		t.Fatalf("reserve duplicate simplified key: %v", err)
	}
	if created {
		t.Fatal("last-two-label key co.uk must remain unique")
	}
}

func TestSQLClaimStoreMigratesLegacyJSON(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "claims.db")
	legacy := map[string]claimInfo{
		"example.com": {
			PrimaryDomain: "example.com",
			Domain:        "www.example.com",
			Wallet:        "congrid1legacy",
			TxHash:        "ABC123",
			ClaimedAt:     time.Unix(1_700_000_000, 0).UTC(),
		},
	}
	b, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, b, 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := openSQLClaimStore(context.Background(), "sqlite", "", dbPath)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	claim, ok, err := store.Get(context.Background(), "example.com")
	if err != nil || !ok {
		t.Fatalf("get migrated claim: ok=%t err=%v", ok, err)
	}
	if claim.Status != claimStatusConfirmed || claim.TxHash != "ABC123" {
		t.Fatalf("unexpected migrated claim: status=%s tx=%s", claim.Status, claim.TxHash)
	}
	if _, err := os.Stat(dbPath + ".json.bak"); err != nil {
		t.Fatalf("legacy backup missing: %v", err)
	}
}

func TestSQLClaimStoreRestoresLegacyJSONWhenImportFails(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "claims.db")
	legacy := map[string]claimInfo{
		"example.com": {
			PrimaryDomain: "example.com",
			Domain:        "different.net",
			Wallet:        "congrid1legacy",
			TxHash:        "ABC123",
		},
	}
	b, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbPath, b, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := openSQLClaimStore(context.Background(), "sqlite", "", dbPath); err == nil {
		t.Fatal("inconsistent legacy claim must fail migration")
	}
	restored, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("legacy file was not restored: %v", err)
	}
	if string(restored) != string(b) {
		t.Fatal("restored legacy file content changed")
	}
	if _, err := os.Stat(dbPath + ".json.bak"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unexpected backup after rollback: %v", err)
	}
}

func TestAmbiguousBroadcastIsNeverRetried(t *testing.T) {
	store := openTestClaimStore(t)
	claim, created, err := store.Reserve(context.Background(), testClaim("example.com", "congrid1wallet"))
	if err != nil || !created {
		t.Fatalf("reserve claim: created=%t err=%v", created, err)
	}
	fake := &fakeAirdropTxClient{broadcastErr: errors.New("connection reset"), ambiguous: true}
	air := &airdropper{store: store, tx: fake}

	processed, err := air.submitOne(context.Background())
	if !processed || err == nil {
		t.Fatalf("ambiguous submission: processed=%t err=%v", processed, err)
	}
	stored, ok, err := store.Get(context.Background(), claim.PrimaryDomain)
	if err != nil || !ok {
		t.Fatalf("get claim: ok=%t err=%v", ok, err)
	}
	if stored.Status != claimStatusNeedsReconcile {
		t.Fatalf("unexpected status: %s", stored.Status)
	}

	processed, err = air.submitOne(context.Background())
	if err != nil || processed {
		t.Fatalf("claim should not be retried: processed=%t err=%v", processed, err)
	}
	if fake.broadcastCalls != 1 {
		t.Fatalf("unexpected broadcast calls: %d", fake.broadcastCalls)
	}
}

func TestBroadcastThenConfirmStateMachine(t *testing.T) {
	store := openTestClaimStore(t)
	_, created, err := store.Reserve(context.Background(), testClaim("example.com", "congrid1wallet"))
	if err != nil || !created {
		t.Fatalf("reserve claim: created=%t err=%v", created, err)
	}
	fake := &fakeAirdropTxClient{txHash: "TX123", confirmation: txConfirmationConfirmed}
	air := &airdropper{store: store, tx: fake}

	processed, err := air.submitOne(context.Background())
	if err != nil || !processed {
		t.Fatalf("submit claim: processed=%t err=%v", processed, err)
	}
	if err := air.confirmOne(context.Background()); err != nil {
		t.Fatalf("confirm claim: %v", err)
	}
	stored, ok, err := store.Get(context.Background(), "example.com")
	if err != nil || !ok {
		t.Fatalf("get confirmed claim: ok=%t err=%v", ok, err)
	}
	if stored.Status != claimStatusConfirmed || stored.TxHash != "TX123" {
		t.Fatalf("unexpected confirmed claim: status=%s tx=%s", stored.Status, stored.TxHash)
	}
}

func TestUnconfirmedBroadcastMovesToReconciliationWithoutResend(t *testing.T) {
	store := openTestClaimStore(t)
	_, created, err := store.Reserve(context.Background(), testClaim("example.com", "congrid1wallet"))
	if err != nil || !created {
		t.Fatalf("reserve claim: created=%t err=%v", created, err)
	}
	if transitioned, err := store.MarkSubmitting(context.Background(), "example.com"); err != nil || !transitioned {
		t.Fatalf("mark submitting: transitioned=%t err=%v", transitioned, err)
	}
	if err := store.MarkBroadcast(context.Background(), "example.com", "TX123", time.Now().Add(-2*time.Minute)); err != nil {
		t.Fatalf("mark broadcast: %v", err)
	}
	fake := &fakeAirdropTxClient{confirmation: txConfirmationPending}
	air := &airdropper{store: store, tx: fake, cfg: airdropConfig{ConfirmTimeout: time.Minute}}

	if err := air.confirmOne(context.Background()); err != nil {
		t.Fatalf("confirm pending claim: %v", err)
	}
	stored, ok, err := store.Get(context.Background(), "example.com")
	if err != nil || !ok {
		t.Fatalf("get reconciled claim: ok=%t err=%v", ok, err)
	}
	if stored.Status != claimStatusNeedsReconcile {
		t.Fatalf("unexpected status: %s", stored.Status)
	}
	processed, err := air.submitOne(context.Background())
	if err != nil || processed {
		t.Fatalf("uncertain transaction must not be resent: processed=%t err=%v", processed, err)
	}
}

func TestVerificationNetworkPolicy(t *testing.T) {
	tests := []struct {
		address string
		public  bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"169.254.169.254", false},
		{"100.64.0.1", false},
		{"192.0.2.1", false},
		{"::1", false},
		{"2001:db8::1", false},
	}
	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			ip := mustParseAddr(t, tt.address)
			if got := isPublicUnicast(ip); got != tt.public {
				t.Fatalf("isPublicUnicast(%s)=%t want %t", ip, got, tt.public)
			}
		})
	}
}

func TestVerificationRedirectMustStayOnOriginalHost(t *testing.T) {
	policy := secureRedirectPolicy(false)
	originalURL, _ := url.Parse("https://www.example.com/")
	original := &http.Request{URL: originalURL}

	allowedURL, _ := url.Parse("https://example.com/start")
	if err := policy(&http.Request{URL: allowedURL}, []*http.Request{original}); err != nil {
		t.Fatalf("www redirect rejected: %v", err)
	}

	blockedURL, _ := url.Parse("https://docs.example.com/")
	if err := policy(&http.Request{URL: blockedURL}, []*http.Request{original}); err == nil {
		t.Fatal("cross-host redirect must be rejected")
	}
}

func TestProductionVerificationOnlyAllowsDefaultHTTPSPort(t *testing.T) {
	if err := validatePublicWebsiteDomain("example.com", false); err != nil {
		t.Fatalf("default HTTPS domain rejected: %v", err)
	}
	if err := validatePublicWebsiteDomain("example.com:443", false); err != nil {
		t.Fatalf("explicit HTTPS port rejected: %v", err)
	}
	if err := validatePublicWebsiteDomain("example.com:8080", false); err == nil {
		t.Fatal("non-HTTPS production port must be rejected")
	}
	if err := validatePublicWebsiteDomain("example.com:8080", true); err != nil {
		t.Fatalf("development port rejected: %v", err)
	}
}

func TestDecodeTxResponseWithGasEstimatePrefix(t *testing.T) {
	out := []byte("gas estimate: 90000\n{\"txhash\":\"ABC\",\"code\":0,\"raw_log\":\"\"}\n")
	resp, err := decodeTxResponse(out)
	if err != nil {
		t.Fatal(err)
	}
	if resp.TxHash != "ABC" || resp.Code != 0 {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func openTestClaimStore(t *testing.T) *sqlClaimStore {
	t.Helper()
	store, err := openSQLClaimStore(context.Background(), "sqlite", "", filepath.Join(t.TempDir(), "claims.sqlite"))
	if err != nil {
		t.Fatalf("open test claim store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func testClaim(siteKey, wallet string) claimInfo {
	return claimInfo{
		PrimaryDomain: siteKey,
		Domain:        siteKey,
		Wallet:        wallet,
		Amount:        "25000",
		Denom:         "ucongrid",
	}
}

type fakeAirdropTxClient struct {
	txHash         string
	broadcastErr   error
	ambiguous      bool
	confirmation   txConfirmation
	broadcastCalls int
}

func (f *fakeAirdropTxClient) Broadcast(context.Context, claimInfo) (string, bool, error) {
	f.broadcastCalls++
	return f.txHash, f.ambiguous, f.broadcastErr
}

func (f *fakeAirdropTxClient) Query(context.Context, string) (txConfirmation, string, error) {
	return f.confirmation, "", nil
}

func mustParseAddr(t *testing.T, value string) netip.Addr {
	t.Helper()
	addr, err := netip.ParseAddr(value)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}
