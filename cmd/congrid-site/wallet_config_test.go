package main

import "testing"

func TestResolveWalletEndpointsRewritesLocalNodeToBaseURLHost(t *testing.T) {
	t.Parallel()

	rpc, rest, err := resolveWalletEndpoints(
		"https://congrid.net",
		"tcp://127.0.0.1:26657",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("resolveWalletEndpoints returned error: %v", err)
	}
	if rpc != "http://congrid.net:26657" {
		t.Fatalf("unexpected rpc: %s", rpc)
	}
	if rest != "http://congrid.net:1317" {
		t.Fatalf("unexpected rest: %s", rest)
	}
}

func TestResolveWalletEndpointsRespectsExplicitWalletEndpoints(t *testing.T) {
	t.Parallel()

	rpc, rest, err := resolveWalletEndpoints(
		"https://congrid.net",
		"tcp://127.0.0.1:26657",
		"http://rpc.congrid.net:26657",
		"http://api.congrid.net:1317",
	)
	if err != nil {
		t.Fatalf("resolveWalletEndpoints returned error: %v", err)
	}
	if rpc != "http://rpc.congrid.net:26657" {
		t.Fatalf("unexpected rpc: %s", rpc)
	}
	if rest != "http://api.congrid.net:1317" {
		t.Fatalf("unexpected rest: %s", rest)
	}
}

func TestResolveWalletEndpointsRewritesLocalhostName(t *testing.T) {
	t.Parallel()

	rpc, rest, err := resolveWalletEndpoints(
		"https://congrid.net",
		"http://localhost:26657",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("resolveWalletEndpoints returned error: %v", err)
	}
	if rpc != "http://congrid.net:26657" {
		t.Fatalf("unexpected rpc: %s", rpc)
	}
	if rest != "http://congrid.net:1317" {
		t.Fatalf("unexpected rest: %s", rest)
	}
}
