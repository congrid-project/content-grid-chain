package executor

import (
	"errors"
	"regexp"
	"strings"

	htmlmd "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
)

// Document represents normalized content derived from a fetched asset.
type Document struct {
	Title    string
	Markdown string
}

var whitespaceCollapse = regexp.MustCompile(`\s+`)

// PreprocessDocument normalizes raw bytes into markdown suitable for embedding/classification.
// For HTML documents this removes boilerplate (headers, footers, sidebars, scripts), extracts the
// page title, strips Content Grid specific code snippets, and converts the remaining structure to markdown.
func PreprocessDocument(raw []byte) (Document, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return Document{}, errors.New("empty document")
	}
	if !looksLikeHTML(trimmed) {
		normalized := collapseWhitespace(trimmed)
		return Document{Markdown: normalized}, nil
	}
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(trimmed))
	if err != nil {
		return Document{}, err
	}
	pruneHTML(doc)
	title := strings.TrimSpace(doc.Find("title").First().Text())
	if title == "" {
		title = strings.TrimSpace(doc.Find("h1").First().Text())
	}
	mainSel := doc.Find("main")
	if mainSel.Length() == 0 {
		mainSel = doc.Find("article")
	}
	if mainSel.Length() == 0 {
		mainSel = doc.Find("body")
	}
	html, err := mainSel.Html()
	if err != nil {
		return Document{}, err
	}
	converter := htmlmd.NewConverter("", true, nil)
	markdown, err := converter.ConvertString(html)
	if err != nil {
		return Document{}, err
	}
	markdown = normalizeMarkdown(markdown)
	if title != "" {
		markdown = "# " + title + "\n\n" + markdown
	}
	return Document{Title: title, Markdown: markdown}, nil
}

func looksLikeHTML(input string) bool {
	lowered := strings.ToLower(input)
	return strings.Contains(lowered, "<html") || strings.Contains(lowered, "<body") || strings.Contains(lowered, "<!doctype")
}

func collapseWhitespace(input string) string {
	normalized := whitespaceCollapse.ReplaceAllString(input, " ")
	normalized = strings.TrimSpace(normalized)
	return normalized
}

func normalizeMarkdown(input string) string {
	// normalise line endings and trim outer whitespace while preserving structure
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	lines := strings.Split(input, "\n")
	var out []string
	blank := false
	for _, line := range lines {
		trimmedRight := strings.TrimRight(line, " \t")
		if strings.TrimSpace(trimmedRight) == "" {
			if !blank {
				out = append(out, "")
				blank = true
			}
			continue
		}
		blank = false
		out = append(out, trimmedRight)
	}
	// remove leading/trailing blanks
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

func pruneHTML(doc *goquery.Document) {
	// Remove scripts, styles, and hidden sections.
	doc.Find("script, style, template, noscript, svg, canvas").Remove()

	// Remove header/footer/navigation/aside sections and obvious sidebar containers.
	doc.Find("header, footer, nav, aside, [role='navigation'], [role='banner'], [role='complementary']").Remove()
	doc.Find(".sidebar, .side-bar, .drawer, .offcanvas").Each(func(_ int, sel *goquery.Selection) {
		sel.Remove()
	})

	// Remove code snippets mentioning Content Grid
	doc.Find("pre, code").Each(func(_ int, sel *goquery.Selection) {
		text := strings.ToLower(sel.Text())
		if strings.Contains(text, "content grid") {
			sel.Remove()
		}
	})

	// Drop common layout elements by id/class names
	doc.Find("#header, #footer, #sidebar, .site-header, .site-footer").Remove()

	// Remove empty elements
	doc.Find("p, div, section").Each(func(_ int, sel *goquery.Selection) {
		content := strings.TrimSpace(sel.Text())
		if content == "" && sel.Children().Length() == 0 {
			sel.Remove()
		}
	})
}

// buildEmbeddingInput constructs the text used by embedding/classification from the processed document.
func buildEmbeddingInput(doc Document) []byte {
	if doc.Markdown == "" {
		return nil
	}
	return []byte(doc.Markdown)
}
