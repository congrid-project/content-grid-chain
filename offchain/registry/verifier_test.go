package registryoffchain

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestHTTPContentVerifier(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr bool
	}{
		{
			name:    "no congrid links",
			body:    `<a href="https://example.com">Example</a>`,
			wantErr: true,
		},
		{
			name:    "single congrid link without wallet binding",
			body:    `<a href="https://congrid.net">Congrid</a>`,
			wantErr: true,
		},
		{
			name:    "official anchor + img with wallet query matches owner",
			body:    `<a href="https://congrid.net"><img src="https://congrid.net/badge.svg?publisher={{publisher}}&wallet=cosmos1owner" /></a>`,
			wantErr: false,
		},
		{
			name:    "official badge publisher must match registered domain",
			body:    `<a href="https://congrid.net"><img src="https://congrid.net/badge.svg?publisher=wrong.example&wallet=cosmos1owner" /></a>`,
			wantErr: true,
		},
		{
			name:    "official anchor + img with data-wallet style (not supported) should fail",
			body:    `<a href="https://congrid.net" data-wallet="cosmos1owner"><img src="https://congrid.net/badge.png?publisher=example.com" /></a>`,
			wantErr: true,
		},
		{
			name:    "official anchor + img with mismatched wallet",
			body:    `<a href="https://congrid.net"><img src="https://congrid.net/badge.png?publisher={{publisher}}&wallet=cosmos1notowner" /></a>`,
			wantErr: true,
		},
		{
			name:    "official anchor must have no query",
			body:    `<a href="https://congrid.net/?x=1"><img src="https://congrid.net/badge.png?publisher={{publisher}}&wallet=cosmos1owner" /></a>`,
			wantErr: true,
		},
		{
			name:    "missing img",
			body:    `<a href="https://congrid.net">Verified</a>`,
			wantErr: true,
		},
		{
			name:    "missing publisher info in img",
			body:    `<a href="https://congrid.net"><img src="https://congrid.net/badge.png?wallet=cosmos1owner" /></a>`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != DefaultVerifyPath {
					http.NotFound(w, r)
					return
				}
				_, _ = w.Write([]byte(body))
			}))
			t.Cleanup(server.Close)

			parsed, err := url.Parse(server.URL)
			if err != nil {
				t.Fatalf("failed to parse server url: %v", err)
			}

			body = strings.ReplaceAll(tt.body, "{{publisher}}", parsed.Host)

			verifier := HTTPContentVerifier{Scheme: parsed.Scheme, Client: server.Client()}
			err = verifier.Verify(context.Background(), parsed.Host, "cosmos1owner")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected verification to fail")
				}
				return
			}
			if err != nil {
				t.Fatalf("expected verification to pass: %v", err)
			}
		})
	}
}

func TestEnsureLeaseAnchors_RequiresWrapperSlotID(t *testing.T) {
	html := `
<div data-congrid-slot-id="slot-000123">
  <a href="https://advertiser.example/landing" data-congrid-lease="lease-000456">Sponsored</a>
</div>`
	err := ensureLeaseAnchors(html, []LeaseExpectation{{
		SlotID:    "slot-000123",
		LeaseID:   "lease-000456",
		TargetURL: "https://advertiser.example/landing",
	}})
	if err != nil {
		t.Fatalf("expected lease anchors to pass: %v", err)
	}
}

func TestEnsureLeaseAnchors_FailsWhenWrapperSlotIDMissing(t *testing.T) {
	html := `<a href="https://advertiser.example/landing" data-congrid-slot="slot-000123" data-congrid-lease="lease-000456">Sponsored</a>`
	err := ensureLeaseAnchors(html, []LeaseExpectation{{
		SlotID:    "slot-000123",
		LeaseID:   "lease-000456",
		TargetURL: "https://advertiser.example/landing",
	}})
	if err == nil {
		t.Fatalf("expected lease anchor verification to fail without wrapper slot id")
	}
}

func TestHTTPContentVerifier_VerifyWithLeases(t *testing.T) {
	body := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != DefaultVerifyPath {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("failed to parse server url: %v", err)
	}

	badge := fmt.Sprintf(`<a href="https://congrid.net"><img src="https://congrid.net/badge.png?publisher=%s&wallet=cosmos1owner" /></a>`, parsed.Host)
	lease := `<div data-congrid-slot-id="slot-000123"><a href="https://advertiser.example/landing" data-congrid-lease="lease-000456">Sponsored</a></div>`
	body = badge + lease

	verifier := HTTPContentVerifier{Scheme: parsed.Scheme, Client: server.Client()}
	err = verifier.VerifyWithLeases(context.Background(), parsed.Host, "cosmos1owner", []LeaseExpectation{{
		SlotID:    "slot-000123",
		LeaseID:   "lease-000456",
		TargetURL: "https://advertiser.example/landing",
	}})
	if err != nil {
		t.Fatalf("expected VerifyWithLeases to pass: %v", err)
	}
}
