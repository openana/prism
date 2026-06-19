// LLM usage: generated with deepseek-v4-pro and modified manually.
package web

import (
	"bytes"
	"html/template"
	"io/fs"
	"strings"
	"testing"

	"github.com/openana/prism/pkg/web/i18n"
)

// stubSite returns a minimal Site for template funcMap (used by base.html header).
func stubSite() *Site {
	return &Site{
		Name: "Test Mirror",
		URL:  "https://mirrors.example.com",
	}
}

// loadedHelp bundles a pre-parsed help template with the data needed to render it.
type loadedHelp struct {
	tpl   *template.Template
	cname string
	data  HelpPageData
}

// loadHelpTemplates reads, parses, and prepares every .html file in helpFS.
// The returned slice is suitable for both one-shot smoke tests and benchmarks.
func loadHelpTemplates(tb testing.TB) []loadedHelp {
	tb.Helper()

	funcMap := newTemplateFuncMap(stubSite)
	locale := i18n.Default()
	helpLinks := []HelpLink{{Cname: "test", URL: "/help/test"}}

	entries, err := fs.ReadDir(helpFS, ".")
	if err != nil {
		tb.Fatalf("failed to read helpFS: %v", err)
	}

	var loaded []loadedHelp
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}
		name := entry.Name()
		cname, lang := parseHelpFilename(name)
		if cname == "" {
			tb.Errorf("%s: could not parse cname from filename", name)
			continue
		}

		tpl := template.New(name).Funcs(funcMap)
		if _, err := tpl.ParseFS(templateFS, "base.html", "help_layout.html"); err != nil {
			tb.Fatalf("%s: parse base/help_layout: %v", name, err)
		}
		if _, err := tpl.ParseFS(helpFS, name); err != nil {
			tb.Fatalf("%s: parse help template: %v", name, err)
		}

		loaded = append(loaded, loadedHelp{
			tpl:   tpl,
			cname: cname,
			data: HelpPageData{
				PageBase: PageBase{
					Title:    "help.title",
					Locale:   resolveTemplateLocale(locale, lang),
					PageType: PageTypeHelp,
				},
				Endpoint:  "https://mirrors.example.com/" + cname,
				HelpLinks: helpLinks,
				Cname:     cname,
			},
		})
	}

	if len(loaded) == 0 {
		tb.Fatal("no .html files found in helpFS — is templates/help/ populated?")
	}

	return loaded
}

func TestHelpTemplatesParseAndRender(t *testing.T) {
	loaded := loadHelpTemplates(t)

	for _, l := range loaded {
		t.Run(l.cname, func(t *testing.T) {
			var buf bytes.Buffer
			if err := l.tpl.ExecuteTemplate(&buf, "base", l.data); err != nil {
				t.Fatalf("render: %v", err)
			}

			if buf.Len() == 0 {
				t.Error("rendered output is empty")
			}

			// Quick sanity: output should contain the cname somewhere.
			if !strings.Contains(buf.String(), l.cname) {
				t.Logf("note: output does not contain cname %q (this may be fine)", l.cname)
			}
		})
	}

	t.Logf("tested %d help templates", len(loaded))
}

// parseHelpFilename extracts cname and lang from a help template filename.
// e.g. "alpine.zh.html" -> ("alpine", "zh")
// "linux-stable.git.zh.html" -> ("linux-stable.git", "zh")
func parseHelpFilename(name string) (cname, lang string) {
	// Strip ".html" suffix.
	base := strings.TrimSuffix(name, ".html")
	// Find the last dot — what follows is the language tag.
	idx := strings.LastIndex(base, ".")
	if idx == -1 {
		return "", ""
	}
	cname = base[:idx]
	lang = base[idx+1:]
	return cname, lang
}

// resolveTemplateLocale returns a *i18n.Locale for the given language tag.
// If the template is "zh", we use the default locale (en) since zh translations
// may not exist — the .T method falls back to the key itself which is fine for
// a parse/render smoke test. If we wanted full locale fidelity we'd need to
// load zh-CN, but the i18n package only has en.yaml loaded by default.
func resolveTemplateLocale(defaultLocale *i18n.Locale, lang string) *i18n.Locale {
	// For now, all help templates are zh. The default locale (en) works because
	// .T falls back to the key string when no translation exists, which is
	// sufficient for a parse+render smoke test.
	_ = lang
	return defaultLocale
}

// Benchmark all help templates in one batch — useful for tracking regressions
// in template rendering performance.
func BenchmarkHelpTemplatesRender(b *testing.B) {
	loaded := loadHelpTemplates(b)

	b.ResetTimer()
	for range b.N {
		for _, l := range loaded {
			var buf bytes.Buffer
			if err := l.tpl.ExecuteTemplate(&buf, "base", l.data); err != nil {
				b.Fatalf("%s: render: %v", l.cname, err)
			}
		}
	}
}

func TestParseHelpFilename(t *testing.T) {
	tests := []struct {
		name      string
		wantCname string
		wantLang  string
	}{
		{"alpine.zh.html", "alpine", "zh"},
		{"linux-stable.git.zh.html", "linux-stable.git", "zh"},
		{"crates.io-index.git.zh.html", "crates.io-index.git", "zh"},
		{"archlinux.zh.html", "archlinux", "zh"},
	}

	for _, tt := range tests {
		cname, lang := parseHelpFilename(tt.name)
		if cname != tt.wantCname || lang != tt.wantLang {
			t.Errorf("parseHelpFilename(%q) = (%q, %q), want (%q, %q)",
				tt.name, cname, lang, tt.wantCname, tt.wantLang)
		}
	}
}
