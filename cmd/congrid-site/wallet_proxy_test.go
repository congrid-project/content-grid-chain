package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestDeriveWalletProxyEndpointPreservesBasePath(t *testing.T) {
	t.Parallel()

	got := deriveWalletProxyEndpoint("https://congrid.net/app", walletRPCProxyPath)
	want := "https://congrid.net/app/rpc"
	if got != want {
		t.Fatalf("unexpected proxy endpoint: got %q want %q", got, want)
	}
}

func TestNewWalletEndpointProxyStripsPrefixAndKeepsQuery(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.RequestURI()))
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream url: %v", err)
	}

	proxy := newWalletEndpointProxy(walletRPCProxyPath, target)
	server := httptest.NewServer(proxy)
	defer server.Close()

	resp, err := http.Get(server.URL + "/rpc/status?x=1")
	if err != nil {
		t.Fatalf("proxy get: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if got, want := string(body), "/status?x=1"; got != want {
		t.Fatalf("unexpected upstream request uri: got %q want %q", got, want)
	}
}

func TestNewWalletEndpointProxyMapsPrefixRootToUpstreamRoot(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(r.URL.RequestURI()))
	}))
	defer upstream.Close()

	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatalf("parse upstream url: %v", err)
	}

	proxy := newWalletEndpointProxy(walletRESTProxyPath, target)
	server := httptest.NewServer(proxy)
	defer server.Close()

	resp, err := http.Get(server.URL + "/rest")
	if err != nil {
		t.Fatalf("proxy get: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if got, want := string(body), "/"; got != want {
		t.Fatalf("unexpected upstream request uri: got %q want %q", got, want)
	}
}
