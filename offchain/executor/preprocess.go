package executor

import (
	"bytes"
	"fmt"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/PuerkitoBio/goquery"
)

// ProcessedDocument is the normalized representation of an HTML document.
type ProcessedDocument struct {
	Title    string
	Markdown string
}

// PreprocessDocument extracts a title and markdown body from a raw HTML document.
func PreprocessDocument(raw []byte) (ProcessedDocument, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return ProcessedDocument{}, fmt.Errorf("empty document")
	}

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(raw))
	if err != nil {
		// Best-effort fallback for non-HTML input.
		return ProcessedDocument{Markdown: text}, nil
	}

	// Drop noisy nodes before conversion.
	doc.Find("script, style, noscript").Each(func(_ int, s *goquery.Selection) {
		s.Remove()
	})

	title := strings.TrimSpace(doc.Find("title").First().Text())
	bodyHTML, err := doc.Find("body").First().Html()
	if err != nil || strings.TrimSpace(bodyHTML) == "" {
		bodyHTML, _ = doc.Html()
	}

	converter := htmltomarkdown.NewConverter("", true, nil)
	markdown, err := converter.ConvertString(bodyHTML)
	if err != nil {
		markdown = strings.TrimSpace(doc.Text())
	} else {
		markdown = strings.TrimSpace(markdown)
		if markdown == "" {
			markdown = strings.TrimSpace(doc.Text())
		}
	}
	if markdown == "" {
		return ProcessedDocument{}, fmt.Errorf("empty document after preprocessing")
	}

	return ProcessedDocument{Title: title, Markdown: markdown}, nil
}
