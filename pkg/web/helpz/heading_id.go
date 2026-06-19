package main

import (
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// headingIDPattern matches " {#some-id}" at the end of heading text.
var headingIDPattern = regexp.MustCompile(`\s+\{#([^}]+)\}\s*$`)

// headingIDTransformer is a goldmark AST transformer that extracts
// `{#id}` custom heading IDs from heading text.
type headingIDTransformer struct{}

var _ parser.ASTTransformer = (*headingIDTransformer)(nil)

func (t *headingIDTransformer) Transform(doc *ast.Document, reader text.Reader, pc parser.Context) {
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		heading, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		// Get the text content of the heading
		textContent := extractHeadingText(heading, reader.Source())
		match := headingIDPattern.FindStringSubmatch(textContent)
		if match == nil {
			return ast.WalkContinue, nil
		}
		id := match[1]

		// Strip the {#id} suffix from the last text node
		stripHeadingID(heading, reader.Source(), match[0])

		// Set the id attribute on the heading
		heading.SetAttributeString("id", id)

		return ast.WalkContinue, nil
	})
}

// extractHeadingText extracts the plain text content of a heading node.
func extractHeadingText(heading *ast.Heading, source []byte) string {
	var parts []string
	for child := heading.FirstChild(); child != nil; child = child.NextSibling() {
		if text, ok := child.(*ast.Text); ok {
			parts = append(parts, string(text.Segment.Value(source)))
		}
	}
	return strings.Join(parts, "")
}

// stripHeadingID removes the {#id} suffix from the heading's last text node
// by trimming the Segment's Stop position in the source.
func stripHeadingID(heading *ast.Heading, source []byte, suffix string) {
	// Find the last text child
	var lastText *ast.Text
	for child := heading.LastChild(); child != nil; child = child.PreviousSibling() {
		if t, ok := child.(*ast.Text); ok {
			lastText = t
			break
		}
	}
	if lastText == nil {
		return
	}

	textContent := string(lastText.Segment.Value(source))
	newContent := strings.TrimSuffix(textContent, suffix)
	// Also trim trailing whitespace that was before {#id}
	newContent = strings.TrimRight(newContent, " ")

	// Calculate the trimmed length and adjust the segment
	trimmedLen := len(textContent) - len(newContent)
	seg := lastText.Segment
	seg = seg.WithStop(seg.Stop - trimmedLen)
	lastText.Segment = seg
}

// headingIDExtension is a goldmark extension that adds the heading ID transformer.
type headingIDExtension struct{}

var _ goldmark.Extender = (*headingIDExtension)(nil)

func (e *headingIDExtension) Extend(m goldmark.Markdown) {
	m.Parser().AddOptions(
		parser.WithASTTransformers(
			util.Prioritized(&headingIDTransformer{}, 100),
		),
	)
}
