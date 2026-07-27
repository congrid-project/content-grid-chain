package main

import (
	"bytes"
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
	require.Contains(t, body, "congrid-node congrid-chroma congrid-indexer congrid-verifier")
	require.Contains(t, body, "/downloads/install.sh")
	require.NotContains(t, body, "verifier-oneclick.sh")
	require.NotContains(t, body, ".env.verifier")
}
