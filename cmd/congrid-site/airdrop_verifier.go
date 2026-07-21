package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	registryoffchain "content-grid-chain/offchain/registry"
	registry "content-grid-chain/x/registry"
)

type homepageVerifier interface {
	Verify(context.Context, string, string) error
}

type secureHomepageVerifier struct {
	client    *http.Client
	allowHTTP bool
}

func newSecureHomepageVerifier(allowHTTP, allowPrivateTargets bool) homepageVerifier {
	transport := &http.Transport{
		// Verification must connect to the address that was resolved and checked
		// below. An environment proxy could otherwise reach private services on
		// behalf of this process and bypass the SSRF checks.
		Proxy:                  nil,
		DialContext:            safeDialContext(allowPrivateTargets),
		ForceAttemptHTTP2:      true,
		MaxIdleConns:           32,
		MaxIdleConnsPerHost:    2,
		IdleConnTimeout:        30 * time.Second,
		TLSHandshakeTimeout:    5 * time.Second,
		ResponseHeaderTimeout:  5 * time.Second,
		ExpectContinueTimeout:  time.Second,
		MaxResponseHeaderBytes: 64 << 10,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
	client.CheckRedirect = secureRedirectPolicy(allowHTTP)
	return &secureHomepageVerifier{client: client, allowHTTP: allowHTTP}
}

func (v *secureHomepageVerifier) Verify(ctx context.Context, domain, wallet string) error {
	if err := validatePublicWebsiteDomain(domain, v.allowHTTP); err != nil {
		return err
	}
	verifier := registryoffchain.HTTPContentVerifier{Client: v.client, Scheme: "https"}
	if err := verifier.Verify(ctx, domain, wallet); err == nil {
		return nil
	} else if !v.allowHTTP {
		return err
	}
	return (registryoffchain.HTTPContentVerifier{Client: v.client, Scheme: "http"}).Verify(ctx, domain, wallet)
}

func validatePublicWebsiteDomain(domain string, allowHTTP bool) error {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if !registry.IsDomainFormatValid(domain) {
		return fmt.Errorf("invalid website domain")
	}
	if allowHTTP {
		return nil
	}
	if idx := strings.LastIndex(domain, ":"); idx >= 0 && domain[idx+1:] != "443" {
		return fmt.Errorf("only HTTPS port 443 is allowed")
	}
	return nil
}

func secureRedirectPolicy(allowHTTP bool) func(*http.Request, []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return fmt.Errorf("too many verification redirects")
		}
		if len(via) == 0 {
			return fmt.Errorf("redirect source unavailable")
		}
		scheme := strings.ToLower(req.URL.Scheme)
		if scheme != "https" && !(allowHTTP && scheme == "http") {
			return fmt.Errorf("verification redirect uses disallowed scheme %q", scheme)
		}
		if err := validatePublicWebsiteDomain(req.URL.Host, allowHTTP); err != nil {
			return fmt.Errorf("invalid verification redirect: %w", err)
		}
		if req.URL.User != nil {
			return fmt.Errorf("verification redirect contains user information")
		}
		originalHost := hostWithoutPort(via[0].URL.Host)
		redirectHost := hostWithoutPort(req.URL.Host)
		if originalHost != redirectHost && strings.TrimPrefix(originalHost, "www.") != strings.TrimPrefix(redirectHost, "www.") {
			return fmt.Errorf("verification redirect left the original host")
		}
		return nil
	}
}

func hostWithoutPort(host string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if idx := strings.LastIndex(host, ":"); idx >= 0 {
		return host[:idx]
	}
	return host
}

func safeDialContext(allowPrivateTargets bool) func(context.Context, string, string) (net.Conn, error) {
	resolver := net.DefaultResolver
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}

	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("split verification target: %w", err)
		}
		ips, err := resolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, fmt.Errorf("resolve verification target: %w", err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("verification target has no IP addresses")
		}
		for _, ip := range ips {
			if !allowPrivateTargets && !isPublicUnicast(ip) {
				return nil, fmt.Errorf("verification target resolved to disallowed address %s", ip)
			}
		}

		var lastErr error
		for _, ip := range ips {
			conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}
		return nil, fmt.Errorf("connect to verification target: %w", lastErr)
	}
}

func isPublicUnicast(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range disallowedVerificationPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

var disallowedVerificationPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:10::/28"),
	netip.MustParsePrefix("2001:20::/28"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("fec0::/10"),
}
