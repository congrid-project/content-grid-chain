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
	if rpc != "https://congrid.net/rpc" {
		t.Fatalf("unexpected rpc: %s", rpc)
	}
	if rest != "https://congrid.net/rest" {
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

func TestResolveWalletEndpointsDefaultsExplicitHostOnlyEndpointsToBaseURLScheme(t *testing.T) {
	t.Parallel()

	rpc, rest, err := resolveWalletEndpoints(
		"https://congrid.net",
		"tcp://127.0.0.1:26657",
		"rpc.congrid.net:26657",
		"api.congrid.net:1317",
	)
	if err != nil {
		t.Fatalf("resolveWalletEndpoints returned error: %v", err)
	}
	if rpc != "https://rpc.congrid.net:26657" {
		t.Fatalf("unexpected rpc: %s", rpc)
	}
	if rest != "https://api.congrid.net:1317" {
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
	if rpc != "https://congrid.net/rpc" {
		t.Fatalf("unexpected rpc: %s", rpc)
	}
	if rest != "https://congrid.net/rest" {
		t.Fatalf("unexpected rest: %s", rest)
	}
}

func TestResolveWalletEndpointsKeepsHTTPForHTTPBaseURL(t *testing.T) {
	t.Parallel()

	rpc, rest, err := resolveWalletEndpoints(
		"http://localhost:8080",
		"tcp://127.0.0.1:26657",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("resolveWalletEndpoints returned error: %v", err)
	}
	if rpc != "http://localhost:8080/rpc" {
		t.Fatalf("unexpected rpc: %s", rpc)
	}
	if rest != "http://localhost:8080/rest" {
		t.Fatalf("unexpected rest: %s", rest)
	}
}

func TestResolveWalletEndpointsRequiresExplicitEndpointWithoutNodeRPC(t *testing.T) {
	t.Parallel()

	_, _, err := resolveWalletEndpoints(
		"https://congrid.net",
		"",
		"",
		"",
	)
	if err == nil {
		t.Fatal("expected error for missing node rpc and wallet endpoints")
	}
}

func TestResolveWalletEndpointsAllowsExplicitEndpointsWithoutNodeRPC(t *testing.T) {
	t.Parallel()

	rpc, rest, err := resolveWalletEndpoints(
		"https://congrid.net",
		"",
		"https://rpc.congrid.net:26657",
		"",
	)
	if err != nil {
		t.Fatalf("resolveWalletEndpoints returned error: %v", err)
	}
	if rpc != "https://rpc.congrid.net:26657" {
		t.Fatalf("unexpected rpc: %s", rpc)
	}
	if rest != "https://rpc.congrid.net:1317" {
		t.Fatalf("unexpected rest: %s", rest)
	}
}
