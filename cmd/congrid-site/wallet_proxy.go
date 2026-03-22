package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path"
	"strings"
)

const (
	walletRPCProxyPath  = "/rpc"
	walletRESTProxyPath = "/rest"
)

func deriveWalletProxyEndpoint(baseURL, proxyPath string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}

	basePath := strings.TrimSuffix(parsed.Path, "/")
	if basePath == "" {
		basePath = "/"
	}

	parsed.Path = joinURLPath(basePath, proxyPath)
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func isWalletProxyEndpoint(baseURL, raw, proxyPath string) bool {
	expected := deriveWalletProxyEndpoint(baseURL, proxyPath)
	if expected == "" {
		return false
	}
	return normalizeEndpointForCompare(raw) == normalizeEndpointForCompare(expected)
}

func normalizeEndpointForCompare(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimSuffix(parsed.String(), "/")
}

func deriveWalletRESTSiblingPath(rpcPath string) (string, bool) {
	cleaned := path.Clean("/" + strings.TrimPrefix(strings.TrimSpace(rpcPath), "/"))
	if path.Base(cleaned) != strings.TrimPrefix(walletRPCProxyPath, "/") {
		return "", false
	}

	sibling := path.Join(path.Dir(cleaned), strings.TrimPrefix(walletRESTProxyPath, "/"))
	if !strings.HasPrefix(sibling, "/") {
		sibling = "/" + sibling
	}
	return sibling, true
}

func newWalletRPCProxy(nodeRPC string) (http.Handler, error) {
	target, err := walletRPCTarget(nodeRPC)
	if err != nil {
		return nil, err
	}
	return newWalletEndpointProxy(walletRPCProxyPath, target), nil
}

func newWalletRESTProxy(nodeRPC string) (http.Handler, error) {
	target, err := walletRESTTarget(nodeRPC)
	if err != nil {
		return nil, err
	}
	return newWalletEndpointProxy(walletRESTProxyPath, target), nil
}

func walletRPCTarget(nodeRPC string) (*url.URL, error) {
	normalized := normalizeWalletEndpoint(nodeRPC, "http")
	if normalized == "" {
		return nil, fmt.Errorf("node rpc endpoint required for wallet proxy")
	}

	target, err := url.Parse(normalized)
	if err != nil || target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("invalid node rpc endpoint %q", nodeRPC)
	}
	return target, nil
}

func walletRESTTarget(nodeRPC string) (*url.URL, error) {
	target, err := walletRPCTarget(nodeRPC)
	if err != nil {
		return nil, err
	}

	restTarget := *target
	setURLPort(&restTarget, "1317")
	return &restTarget, nil
}

func newWalletEndpointProxy(prefix string, target *url.URL) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetXForwarded()
			pr.SetURL(target)
			pr.Out.URL.Path = joinURLPath(target.Path, stripProxyPrefix(prefix, pr.In.URL.Path))
			pr.Out.URL.RawPath = ""
			pr.Out.Host = pr.Out.URL.Host
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("wallet proxy %s -> %s failed: %v", prefix, target, err)
			http.Error(w, "wallet upstream unavailable", http.StatusBadGateway)
		},
	}
	return proxy
}

func stripProxyPrefix(prefix, requestPath string) string {
	stripped := strings.TrimPrefix(requestPath, prefix)
	if stripped == "" || stripped == "/" {
		return "/"
	}
	if !strings.HasPrefix(stripped, "/") {
		return "/" + stripped
	}
	return stripped
}

func joinURLPath(basePath, extraPath string) string {
	if basePath == "" || basePath == "/" {
		if extraPath == "" {
			return "/"
		}
		return extraPath
	}
	if extraPath == "" || extraPath == "/" {
		if strings.HasSuffix(basePath, "/") {
			return basePath
		}
		return basePath + "/"
	}
	switch {
	case strings.HasSuffix(basePath, "/") && strings.HasPrefix(extraPath, "/"):
		return basePath + extraPath[1:]
	case strings.HasSuffix(basePath, "/") || strings.HasPrefix(extraPath, "/"):
		return basePath + extraPath
	default:
		return basePath + "/" + extraPath
	}
}
