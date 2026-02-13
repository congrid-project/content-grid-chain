package executor

import (
	"strings"
	"testing"
)

func TestPreprocessDocumentHTML(t *testing.T) {
	html := `<!doctype html>
<html>
  <head>
    <title>Sample Page</title>
  </head>
  <body>
    <header>Site Header</header>
    <aside>Navigation</aside>
    <main>
      <h1>Welcome</h1>
      <section>
        <h2>Overview</h2>
        <p>This is the primary content.</p>
      </section>
      <pre><code>// content grid example code
function foo() { return 1 }
</code></pre>
    </main>
    <footer>Footer</footer>
  </body>
</html>`

	doc, err := PreprocessDocument([]byte(html))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.Title != "Sample Page" {
		t.Fatalf("expected title 'Sample Page', got %q", doc.Title)
	}
	if doc.Markdown == "" {
		t.Fatalf("expected markdown output")
	}
	if contains := containsInsensitive(doc.Markdown, "Site Header"); contains {
		t.Fatalf("expected header to be removed: %s", doc.Markdown)
	}
	if contains := containsInsensitive(doc.Markdown, "content grid"); contains {
		t.Fatalf("expected content grid code to be removed: %s", doc.Markdown)
	}
	if !containsInsensitive(doc.Markdown, "# Sample Page") {
		t.Fatalf("expected markdown to contain page title heading: %s", doc.Markdown)
	}
	if !containsInsensitive(doc.Markdown, "## Overview") {
		t.Fatalf("expected markdown to contain h2 heading: %s", doc.Markdown)
	}
}

func TestPreprocessDocumentPlainText(t *testing.T) {
	doc, err := PreprocessDocument([]byte("  simple text  "))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.Markdown != "simple text" {
		t.Fatalf("expected trimmed plain text, got %q", doc.Markdown)
	}
}

func containsInsensitive(haystack, needle string) bool {
	haystackLower := strings.ToLower(haystack)
	needleLower := strings.ToLower(needle)
	return strings.Contains(haystackLower, needleLower)
}
