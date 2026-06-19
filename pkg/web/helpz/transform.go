package main

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// newMarkdown creates a goldmark instance with our custom extensions.
func newMarkdown() goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,         // tables, strikethrough, task lists, etc.
			&headingIDExtension{}, // {#id} custom heading IDs
		),
		goldmark.WithRendererOptions(
			// Allow raw HTML so our ZTMPl/GLOBALMENU comment placeholders
			// pass through goldmark into the output for post-processing.
			html.WithUnsafe(),
		),
	)
}

// transformMarkdown converts a single content block's markdown to HTML.
// It handles the full pipeline:
// 1. Extract {ztmpl} blocks -> placeholders (with context from config)
// 2. Strip {#id} headings + goldmark parse -> HTML
// 3. Rebuild ZTMPl placeholders -> form controls + template scripts + code blocks
func transformMarkdown(content string, config *ZDocConfig, templateCounter, globalMenuCounter *int) ([]ZTMPLBlock, string, error) {
	// Step 1: Extract ztmpl blocks
	blocks, cleaned := extractZTMPL(content, config, templateCounter, globalMenuCounter)

	// Step 2: Goldmark parse -> HTML
	md := newMarkdown()
	var buf bytes.Buffer
	if err := md.Convert([]byte(cleaned), &buf); err != nil {
		return nil, "", err
	}
	htmlContent := buf.String()

	// Step 3: Rebuild ZTMPl placeholders
	htmlContent = rebuildZTMPL(htmlContent, blocks, config)

	// Step 4: Escape any remaining mustache syntax in code blocks and inline code.
	// Goldmark-generated <pre><code> and <code> elements may contain {{endpoint}} etc.
	htmlContent = escapeAllMustache(htmlContent)

	return blocks, htmlContent, nil
}

// escapeAllMustache escapes mustache {{ }} syntax in all code-related elements
// (both <pre><code> blocks and inline <code> tags) to prevent Go's template
// engine from interpreting them as function calls.
func escapeAllMustache(html string) string {
	// Escape inside <pre><code> blocks
	html = codeBlockPat.ReplaceAllStringFunc(html, func(match string) string {
		parts := codeBlockPat.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		return parts[1] + escapeForGoTemplate(parts[2]) + parts[3]
	})
	// Escape inside inline <code> tags (for {ztmpl} inline roles)
	html = inlineCodePat.ReplaceAllStringFunc(html, func(match string) string {
		parts := inlineCodePat.FindStringSubmatch(match)
		if parts == nil {
			return match
		}
		return parts[1] + escapeForGoTemplate(parts[2]) + parts[3]
	})
	return html
}

// codeBlockPat matches <pre><code>...</code></pre> blocks.
var codeBlockPat = regexp.MustCompile(`(<pre><code[^>]*>)([\s\S]*?)(</code></pre>)`)

// inlineCodePat matches inline <code>...</code> tags.
var inlineCodePat = regexp.MustCompile(`(<code[^>]*>)(.*?)(</code>)`)

// transformPage transforms all blocks of a page and concatenates results.
func transformPage(page *Page) error {
	var allHTML []string
	var allZTMPLs []ZTMPLBlock

	tc := 0 // shared template counter across all blocks
	gc := 0 // shared global menu counter across all blocks

	for _, blockContent := range page.Blocks {
		blocks, html, err := transformMarkdown(blockContent, page.Config, &tc, &gc)
		if err != nil {
			return err
		}
		allZTMPLs = append(allZTMPLs, blocks...)
		// Only add non-empty HTML (skip blocks that are pure ztmpl placeholders)
		if strings.TrimSpace(html) != "" {
			allHTML = append(allHTML, html)
		}
	}

	page.ZTMPLs = allZTMPLs
	page.HTML = strings.Join(allHTML, "\n\n")
	return nil
}
