package main

import (
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

// generate walks the source directory, processes all pages, and writes
// Go HTML template files to the output directory.
func generate(srcDir, outDir string) error {
	pages, err := discoverPages(srcDir)
	if err != nil {
		return fmt.Errorf("discovering pages: %w", err)
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	var generated int
	var errors []string

	for _, p := range pages {
		config, err := loadConfig(srcDir, p.CName, p.Locale)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s/%s: %v", p.CName, p.Locale, err))
			continue
		}

		// Load content blocks in order
		var blockContents []string
		for _, blockName := range config.Block {
			content, err := loadBlock(srcDir, p.CName, blockName, p.Locale)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s/%s block %q: %v", p.CName, p.Locale, blockName, err))
				continue
			}
			blockContents = append(blockContents, content)
		}

		if len(blockContents) == 0 {
			errors = append(errors, fmt.Sprintf("%s/%s: no content blocks loaded", p.CName, p.Locale))
			continue
		}

		page := &Page{
			CName:  p.CName,
			Locale: p.Locale,
			Config: config,
			Blocks: blockContents,
		}

		if err := transformPage(page); err != nil {
			errors = append(errors, fmt.Sprintf("%s/%s transform: %v", p.CName, p.Locale, err))
			continue
		}

		outputHTML := buildTemplate(page)

		outFile := filepath.Join(outDir, fmt.Sprintf("%s.%s.html", p.CName, p.Locale))
		if err := os.WriteFile(outFile, []byte(outputHTML), 0o644); err != nil {
			errors = append(errors, fmt.Sprintf("%s/%s write: %v", p.CName, p.Locale, err))
			continue
		}

		generated++
		fmt.Printf("Generated: %s -> %s\n", p.CName, outFile)
	}

	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "\n%d error(s):\n", len(errors))
		for _, e := range errors {
			fmt.Fprintf(os.Stderr, "  - %s\n", e)
		}
	}

	fmt.Printf("\nDone. %d page(s) generated.\n", generated)
	return nil
}

// buildTemplate constructs the final Go HTML template output for a page.
// Output structure:
//
//	{{ define "content" }}
//	<article>
//	  <h1>{{ .Title }}</h1>
//	  <!-- rendered HTML -->
//	  <!-- toggles for sudo (always) and https (unless filter.scheme=="https") under the title -->
//	</article>
//	{{ end }}
func buildTemplate(page *Page) string {
	var sb strings.Builder

	sb.WriteString("{{ define \"help_body\" }}\n")
	// Hidden input supplies endpoint to mirrorz-help.js (mustache.js reads it).
	sb.WriteString("<input data-var=\"endpoint\" data-global value=\"{{ .Endpoint }}\" hidden>\n")
	sb.WriteString("<article>\n")

	// Title
	sb.WriteString(fmt.Sprintf("<h1>%s</h1>\n", html.EscapeString(page.Config.Title)))

	// Fixed toggles right after title
	sb.WriteString(buildFixedToggles(page.Config))

	// Rendered HTML content
	sb.WriteString(page.HTML)
	sb.WriteString("\n")

	sb.WriteString("</article>\n")
	sb.WriteString("{{ end }}\n")

	return sb.String()
}

// buildFixedToggles generates the sudo and https toggle HTML using the
// Pico CSS fieldset sibling pattern: <input> + <label htmlFor="id">.
// HTTPS toggle is omitted if filter.scheme is "https".
func buildFixedToggles(config *ZDocConfig) string {
	var sb strings.Builder

	sb.WriteString(`<fieldset class="fixed-toggles">` + "\n")

	// Sudo toggle — always present
	sb.WriteString(`  <input type="checkbox" role="switch" id="toggle-sudo" class="sudo-toggle" checked />` + "\n")
	sb.WriteString(`  <label htmlFor="toggle-sudo" class="input-toggle">{{ .Locale.T "help.use_sudo" }}</label>` + "\n")

	// HTTPS toggle — only if not forced to https
	if config.Filter == nil || config.Filter.Scheme != "https" {
		sb.WriteString(`  <input type="checkbox" role="switch" id="toggle-https" class="https-toggle" checked />` + "\n")
		sb.WriteString(`  <label htmlFor="toggle-https" class="input-toggle">{{ .Locale.T "help.use_https" }}</label>` + "\n")
	}

	sb.WriteString(`</fieldset>` + "\n")

	return sb.String()
}
