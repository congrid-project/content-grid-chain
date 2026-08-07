package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseObservedSimilarDomainsUsesOnlySimilarContainer(t *testing.T) {
	page := `
<div id="congrid-similar">
  <img src="decorative.svg">
  <a href="https://Alpha.example/path">Alpha</a>
  <div><a href="https://beta.example">Beta</a></div>
  <a href="https://alpha.example/duplicate">Alpha again</a>
</div>
<a href="https://outside.example">Outside</a>`

	domains, err := parseObservedSimilarDomains(page)
	require.NoError(t, err)
	require.Equal(t, []string{"alpha.example", "beta.example"}, domains)
}
