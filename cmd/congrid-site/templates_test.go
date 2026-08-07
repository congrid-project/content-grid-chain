package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerifierPageUsesNativeInstaller(t *testing.T) {
	templates, err := buildPageTemplates(siteFS)
	require.NoError(t, err)

	var rendered bytes.Buffer
	err = templates["verifiers.html"].ExecuteTemplate(&rendered, "verifiers.html", pageData{
		Title:   "Become a Verifier — Congrid",
		BaseURL: "https://congrid.net",
		Path:    "/verifiers",
		WalletConfig: WalletConfig{
			ChainID:  "congrid-main",
			RPC:      "https://congrid.net/rpc",
			FeeDenom: "ucongrid",
			GasPrice: "0.001ucongrid",
		},
	})
	require.NoError(t, err)

	body := rendered.String()
	require.Contains(t, body, "curl -fsSL https://congrid.net/downloads/install.sh | bash")
	require.Contains(t, body, "bash -s -- --components-only")
	require.Contains(t, body, "congrid-node congrid-chroma congrid-indexer congrid-verifier")
	require.Contains(t, body, "/downloads/install.sh")
	require.NotContains(t, body, "verifier-oneclick.sh")
	require.NotContains(t, body, ".env.verifier")
}

func TestHomePageIncludesPublisherVerificationBadge(t *testing.T) {
	templates, err := buildPageTemplates(siteFS)
	require.NoError(t, err)

	var rendered bytes.Buffer
	err = templates["home.html"].ExecuteTemplate(&rendered, "home.html", pageData{
		Title:   "Congrid — Content Grid Protocol",
		BaseURL: "https://congrid.net",
		Path:    "/",
	})
	require.NoError(t, err)

	body := rendered.String()
	require.Contains(t, body, `<a href="https://congrid.net">`)
	require.Contains(t, body, `src="https://congrid.net/badge.svg?publisher=congrid.net&wallet=congrid18cepycc5rv3dpe24n0mmdkdqwaruptvkuuurxf"`)
	require.NotContains(t, body, "congrid1c6vuutzwzq0fxqw8fpscwdytnc08qnfq3ufp2t")
}

func TestBadgeSVGMatchesStaticLogo(t *testing.T) {
	expected, err := siteFS.ReadFile("static/assets/congrid-logo.svg")
	require.NoError(t, err)

	for _, requestPath := range []string{
		"/badge.svg?publisher=example.com&wallet=congrid1owner",
		"/badge.png?publisher=example.com&wallet=congrid1owner",
	} {
		t.Run(requestPath, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, requestPath, nil)

			(&server{}).handleBadgeSVG(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, "image/svg+xml; charset=utf-8", recorder.Header().Get("Content-Type"))
			require.Equal(t, expected, recorder.Body.Bytes())
		})
	}
}

func TestHomePageFooterIncludesSourceCodeLink(t *testing.T) {
	templates, err := buildPageTemplates(siteFS)
	require.NoError(t, err)

	var rendered bytes.Buffer
	err = templates["home.html"].ExecuteTemplate(&rendered, "home.html", pageData{
		Title:   "Congrid — Content Grid Protocol",
		BaseURL: "https://congrid.net",
		Path:    "/",
	})
	require.NoError(t, err)

	body := rendered.String()
	require.Contains(t, body, `href="https://github.com/congrid-project/content-grid-chain"`)
	require.Contains(t, body, `>Source code on GitHub</a>`)
}

func TestPublisherPageSupportsVerifierReferrer(t *testing.T) {
	templates, err := buildPageTemplates(siteFS)
	require.NoError(t, err)

	var rendered bytes.Buffer
	err = templates["publishers.html"].ExecuteTemplate(&rendered, "publishers.html", pageData{
		Title:   "Become a Publisher — Congrid",
		BaseURL: "https://congrid.net",
		Path:    "/publishers",
		WalletConfig: WalletConfig{
			ChainID:  "congrid-main",
			RPC:      "https://congrid.net/rpc",
			FeeDenom: "ucongrid",
			GasPrice: "0.001ucongrid",
		},
	})
	require.NoError(t, err)

	body := rendered.String()
	require.Contains(t, body, `id="publisher-referrer"`)
	require.Contains(t, body, `name="referrer"`)
	require.Contains(t, body, `data-wallet-register-referrer`)
	require.Contains(t, body, "--referrer ${referrer}")
	require.Contains(t, body, "Register / re-register with connected wallet")
	require.Contains(t, body, "can replace or clear this referrer")
}

func TestPublisherPageGeneratesSVGLogoFirstInSimilarSites(t *testing.T) {
	templates, err := buildPageTemplates(siteFS)
	require.NoError(t, err)

	var rendered bytes.Buffer
	err = templates["publishers.html"].ExecuteTemplate(&rendered, "publishers.html", pageData{
		Title:   "Become a Publisher — Congrid",
		BaseURL: "https://congrid.net",
		Path:    "/publishers",
	})
	require.NoError(t, err)

	body := rendered.String()
	require.Contains(t, body, "const badgeSrc = `${baseURL}/badge.svg?publisher=${encodeURIComponent(domain)}&wallet=${encodeURIComponent(wallet)}`;")
	require.Contains(t, body, "const badgeSnippet = `<div id=\"congrid-similar\">\\n  <a href=\"${baseURL}\">")
	require.Contains(t, body, "Add links for all 15 domains returned by indexerd here.")
	require.NotContains(t, body, "/badge.png?publisher=${encodeURIComponent(domain)}")
}

func TestPublisherWalletEncodesReferrerAsProtoFieldSix(t *testing.T) {
	walletJS, err := siteFS.ReadFile("static/wallet.js")
	require.NoError(t, err)

	source := string(walletJS)
	require.Contains(t, source, `writer.uint32(34).string(message.proof)`)
	require.Contains(t, source, `writer.uint32(42).string(message.verifier)`)
	require.Contains(t, source, `writer.uint32(50).string(message.referrer)`)
	require.Contains(t, source, `const referrer = String(data.get("referrer") || "").trim()`)
	require.Contains(t, source, "referrer,\n")
	require.Contains(t, source, `fetch("/publishers/verify"`)
	require.Contains(t, source, `wallet: state.address`)
}
