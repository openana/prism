package main

import (
	"fmt"
	"html"
	"regexp"
	"strings"
)

// ztmplLinePattern matches the opening ```{ztmpl ...}``` line.
// Captures all attributes after "ztmpl". Allows leading whitespace for indented blocks.
var ztmplOpenPattern = regexp.MustCompile(`^\s*\x60{3}\{ztmpl\s*(.*)}\s*$`)

// ztmplClosePattern matches the closing ``` (backticks only, no attributes).
// Allows leading whitespace for indented blocks.
var ztmplClosePattern = regexp.MustCompile(`^\s*\x60{3}\s*$`)

// attrPattern matches key="value" or key=value pairs.
var attrPattern = regexp.MustCompile(`(\w+)\s*=\s*"([^"]*)"`)

// placeholderPrefix marks ZTMPl placeholders in the intermediate text.
// We use ASCII control characters as delimiters: \x01 (SOH) as field separator.
// Goldmark passes these through as-is (they're not HTML and not stripped).
const ztmplPlaceholderPrefix = "\x01ZTMPl"
const globalMenuPrefix = "\x01GLOBALMENU"
const fieldSep = "\x01"

// ztmplPlaceholderPattern matches our SOH-delimited ZTMPl/GLOBALMENU placeholders.
// Format: \x01ZTMPl\x01id\x01lang\x01path\x01append\x01inputs\x01
var ztmplPlaceholderPattern = regexp.MustCompile(
	"\x01(ZTMPl|GLOBALMENU)" +
		"\x01([^\x01]*)" + // id
		"\x01([^\x01]*)" + // lang
		"\x01([^\x01]*)" + // path
		"\x01([^\x01]*)" + // append
		"\x01([^\x01]*)" + // inputs
		"\x01",
)

// extractZTMPL scans markdown text for {ztmpl} fenced blocks,
// replaces them with SOH-delimited placeholders, and returns the
// extracted ZTMPLBlock definitions plus the cleaned markdown.
// templateCounter and globalMenuCounter are shared across all blocks
// of a single page to ensure unique IDs.
func extractZTMPL(markdown string, config *ZDocConfig, templateCounter, globalMenuCounter *int) ([]ZTMPLBlock, string) {
	lines := strings.Split(markdown, "\n")
	var blocks []ZTMPLBlock
	var output []string

	i := 0
	for i < len(lines) {
		line := lines[i]
		match := ztmplOpenPattern.FindStringSubmatch(line)
		if match == nil {
			output = append(output, line)
			i++
			continue
		}

		// Parse attributes
		attrs := parseZTMPLAttrs(match[1])

		// Collect template content until closing ```
		i++
		var templateLines []string
		for i < len(lines) {
			if ztmplClosePattern.MatchString(lines[i]) {
				break
			}
			templateLines = append(templateLines, lines[i])
			i++
		}
		i++ // skip closing ```

		templateContent := strings.Join(templateLines, "\n")

		block := ZTMPLBlock{
			TemplateContent: templateContent,
			Lang:            attrs["lang"],
			Path:            attrs["path"],
			Append:          attrs["append"] == "true",
			Inputs:          strings.Fields(attrs["input"]),
			Global:          attrs["global"] == "true",
		}

		if block.Lang == "" {
			block.Lang = "plaintext"
		}

		if block.Global {
			block.GlobalMenuID = fmt.Sprintf("globalMenu-%d", *globalMenuCounter)
			*globalMenuCounter++
			// Global menu placeholder uses same format as ZTMPl for uniform regex: 7 delimiters
			placeholder := globalMenuPrefix + fieldSep + block.GlobalMenuID + fieldSep +
				fieldSep + fieldSep + fieldSep + // empty lang, path, append
				strings.Join(block.Inputs, " ") + fieldSep
			output = append(output, placeholder)
		} else {
			block.TemplateID = fmt.Sprintf("tmpl-%d", *templateCounter)
			*templateCounter++
			// ZTMPl placeholder: \x01ZTMPl\x01id\x01lang\x01path\x01append\x01inputs\x01
			appendStr := "false"
			if block.Append {
				appendStr = "true"
			}
			placeholder := ztmplPlaceholderPrefix + fieldSep + block.TemplateID + fieldSep +
				block.Lang + fieldSep + block.Path + fieldSep + appendStr + fieldSep +
				strings.Join(block.Inputs, " ") + fieldSep
			output = append(output, placeholder)
		}

		// Validate inputs against config
		for _, inputName := range block.Inputs {
			if _, ok := config.Input[inputName]; !ok {
				// Input not found in config; skip silently (may be adobe-fonts hack)
				continue
			}
		}

		blocks = append(blocks, block)
	}

	return blocks, strings.Join(output, "\n")
}

// parseZTMPLAttrs parses key="value" pairs from a ztmpl attribute string.
func parseZTMPLAttrs(attrStr string) map[string]string {
	attrs := make(map[string]string)
	matches := attrPattern.FindAllStringSubmatch(attrStr, -1)
	for _, m := range matches {
		attrs[m[1]] = m[2]
	}
	return attrs
}

// rebuildZTMPL replaces ZTMPl and GLOBALMENU placeholders in the goldmark-
// rendered HTML with the actual form controls, template scripts, and code blocks.
func rebuildZTMPL(htmlContent string, blocks []ZTMPLBlock, config *ZDocConfig) string {
	// Build lookup maps
	blockMap := make(map[string]*ZTMPLBlock)
	for i := range blocks {
		if !blocks[i].Global {
			blockMap[blocks[i].TemplateID] = &blocks[i]
		}
	}

	result := ztmplPlaceholderPattern.ReplaceAllStringFunc(htmlContent, func(match string) string {
		m := ztmplPlaceholderPattern.FindStringSubmatch(match)
		if m == nil {
			return match
		}
		// m[1]=prefix, m[2]=id, m[3]=lang, m[4]=path, m[5]=append, m[6]=inputs
		prefix := m[1]
		id := m[2]

		if prefix == "GLOBALMENU" {
			return rebuildGlobalMenu(id, m[6], config)
		}
		return rebuildCodeBlock(id, m[3], m[4], m[5], m[6], blockMap, config)
	})

	// Post-process: unwrap <p> tags around generated block elements
	result = unwrapGeneratedBlocks(result)
	return result
}

// unwrapGeneratedBlocks removes <p> wrappers around generated block elements.
var generatedBlockPat = regexp.MustCompile(
	`<p>\s*(<(?:script type="text/x-mustache-template"|pre|fieldset class="(?:code-block-inputs|global-menu)"|div class="file-path")[\s\S]*?</(?:script|pre|fieldset|div)>)\s*</p>`,
)

func unwrapGeneratedBlocks(html string) string {
	return generatedBlockPat.ReplaceAllString(html, "$1")
}

// rebuildGlobalMenu generates HTML for a global menu placeholder.
func rebuildGlobalMenu(id, inputsStr string, config *ZDocConfig) string {
	inputNames := strings.Fields(inputsStr)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<fieldset class="global-menu" data-global-menu="%s">`, id))
	sb.WriteString("\n")
	for _, name := range inputNames {
		sb.WriteString(renderInputControl(name, id, config))
		sb.WriteString("\n")
	}
	sb.WriteString("</fieldset>")
	return sb.String()
}

// rebuildCodeBlock generates HTML for a code block placeholder.
func rebuildCodeBlock(templateID, lang, path, appendStr, inputsStr string, blockMap map[string]*ZTMPLBlock, config *ZDocConfig) string {
	block := blockMap[templateID]
	if block == nil {
		return fmt.Sprintf("<!-- ERROR: unknown template id %s -->", templateID)
	}

	appendFlag := appendStr == "true"
	inputNames := strings.Fields(inputsStr)

	var sb strings.Builder

	// Render input controls above the code block
	if len(inputNames) > 0 {
		sb.WriteString(fmt.Sprintf(`<fieldset class="code-block-inputs" data-template="%s">`, templateID))
		sb.WriteString("\n")
		for _, name := range inputNames {
			sb.WriteString(renderInputControl(name, templateID, config))
			sb.WriteString("\n")
		}
		sb.WriteString("</fieldset>\n")
	}

	// File path indicator
	if path != "" {
		prefix := ""
		if appendFlag {
			prefix = "&gt;&gt; "
		}
		sb.WriteString(fmt.Sprintf(`<div class="file-path" data-template="%s">%s%s</div>`+"\n",
			templateID, prefix, html.EscapeString(path)))
	}

	// Hidden mustache template
	escapedTemplate := escapeForGoTemplate(block.TemplateContent)
	sb.WriteString(fmt.Sprintf(
		`<script type="text/x-mustache-template" id="%s">%s</script>`+"\n",
		templateID, escapedTemplate,
	))

	// Code block target element
	classAttr := ""
	if lang != "" {
		classAttr = fmt.Sprintf(` class="language-%s"`, html.EscapeString(lang))
	}
	sb.WriteString(fmt.Sprintf(
		`<pre><code%s data-template="%s"></code></pre>`+"\n",
		classAttr, templateID,
	))

	return sb.String()
}

// escapeForGoTemplate escapes the entire string for safe embedding inside a
// Go html/template.  The content is destined for a <script> tag on the final
// page; the only Go-template-significant characters outside {{ }} actions are
// the delimiters themselves.  strings.NewReplacer handles all matches in one
// pass without cascading, so we avoid the fragile control-character dance.
func escapeForGoTemplate(s string) string {
	// Escape all Go-template-significant character sequences.
	// Any {{ opens a template action and }} closes it, so both must be quoted.
	return strings.NewReplacer(
		"{{", `{{"{{"}}`,
		"}}", `{{"}}"}}`,
		"<", `{{ "<" }}`,
		">", `{{ ">" }}`,
		"<<", `{{ "<<" }}`,
		">>", `{{ ">>" }}`,
	).Replace(s)
}

// renderInputControl generates the HTML for a single input control using the
// Pico CSS fieldset sibling pattern: <label htmlFor="id"> + <input/select id="id">.
func renderInputControl(name, containerID string, config *ZDocConfig) string {
	def, ok := config.Input[name]
	if !ok {
		return fmt.Sprintf("<!-- ERROR: unknown input %q -->", name)
	}

	ctrlID := containerID + "-" + name
	var sb strings.Builder

	if def.Option != nil {
		// Select dropdown — label before control
		if def.Common.Note != "" {
			sb.WriteString(fmt.Sprintf(`<label htmlFor="%s" data-tooltip="%s">%s</label>`, ctrlID, html.EscapeString(def.Common.Note), html.EscapeString(def.Common.Title)))
		} else {
			sb.WriteString(fmt.Sprintf(`<label htmlFor="%s">%s</label>`, ctrlID, html.EscapeString(def.Common.Title)))
		}
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf(`<select id="%s" class="input-select" data-var="%s">`, ctrlID, html.EscapeString(name)))
		sb.WriteString("\n")
		for _, opt := range def.Option {
			extraJSON := buildExtraJSON(opt.Extra)
			sb.WriteString(fmt.Sprintf(`  <option value="%s" data-extras='%s'>%s</option>`+"\n",
				html.EscapeString(opt.Value), extraJSON, html.EscapeString(opt.Label)))
		}
		sb.WriteString("</select>")
	} else if def.Bool != nil {
		// Boolean toggle / switch — input before label (Pico CSS pattern)
		boolDef := def.Bool
		checked := ""
		if boolDef.Default {
			checked = " checked"
		}
		sb.WriteString(fmt.Sprintf(
			`<input type="checkbox" role="switch" id="%s" class="input-toggle-checkbox" data-var="%s" data-true="%s" data-false="%s"%s />`,
			ctrlID, html.EscapeString(name), html.EscapeString(boolDef.TrueValue), html.EscapeString(boolDef.FalseValue),
			checked,
		))
		sb.WriteString("\n")
		if def.Common.Note != "" {
			sb.WriteString(fmt.Sprintf(`<label htmlFor="%s" class="input-toggle" data-tooltip="%s">%s</label>`, ctrlID, html.EscapeString(def.Common.Note), html.EscapeString(def.Common.Title)))
		} else {
			sb.WriteString(fmt.Sprintf(`<label htmlFor="%s" class="input-toggle">%s</label>`, ctrlID, html.EscapeString(def.Common.Title)))
		}
	} else if def.Text != nil {
		// Text input — label before control
		textDef := def.Text
		if def.Common.Note != "" {
			sb.WriteString(fmt.Sprintf(`<label htmlFor="%s" data-tooltip="%s">%s</label>`, ctrlID, html.EscapeString(def.Common.Note), html.EscapeString(def.Common.Title)))
		} else {
			sb.WriteString(fmt.Sprintf(`<label htmlFor="%s">%s</label>`, ctrlID, html.EscapeString(def.Common.Title)))
		}
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf(
			`<input type="text" id="%s" class="input-text" data-var="%s" value="%s" />`,
			ctrlID, html.EscapeString(name), html.EscapeString(textDef.Default),
		))
	}

	return sb.String()
}

// buildExtraJSON builds a JSON object string from extra key-value pairs.
func buildExtraJSON(extra map[string]string) string {
	if len(extra) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(extra))
	for k, v := range extra {
		parts = append(parts, fmt.Sprintf(`"%s":"%s"`, k, v))
	}
	return "{" + strings.Join(parts, ",") + "}"
}
