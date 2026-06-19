package web

import (
	"bufio"
	"html/template"
	"strings"

	"github.com/valyala/fasthttp"
)

// HelpPageData is the data passed to a help page template.
type HelpPageData struct {
	PageBase
	Endpoint  string
	HelpLinks []HelpLink
	Cname     string
}

// HelpPage bundles locale-specific help templates with the endpoint URL.
type HelpPage struct {
	ByLocale map[string]*template.Template // lang -> template (e.g. "zh" -> tpl)
	Fallback *template.Template            // first available
	Endpoint string
}

// HelpLink describes a help page entry for the sidebar navigation.
type HelpLink struct {
	Cname string
	URL   string
}

func (s *Server) HandleHelpIndex(ctx *fasthttp.RequestCtx) {
	if len(s.help.sorted) == 0 {
		s.handleNotFound(ctx, "/mirrors", "nav.mirrors")
		return
	}
	ctx.Redirect(s.help.sorted[0].URL, fasthttp.StatusFound)
}

func (s *Server) HandleHelp(ctx *fasthttp.RequestCtx) {
	cnameVal, ok := ctx.UserValue("cname").(string)
	if !ok || cnameVal == "" {
		s.handleNotFound(ctx, "/mirrors", "nav.mirrors")
		return
	}

	helpPage, ok := s.pages.help[cnameVal]
	if !ok {
		s.handleNotFound(ctx, "/mirrors", "nav.mirrors")
		return
	}

	// Locale-aware template selection.
	locale := s.resolveLocale(ctx)
	lang := primaryLang(locale.Lang)
	tpl := helpPage.ByLocale[lang]
	if tpl == nil {
		tpl = helpPage.Fallback
	}
	if tpl == nil {
		s.handleNotFound(ctx, "/mirrors", "nav.mirrors")
		return
	}

	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		if err := tpl.ExecuteTemplate(w, "base", HelpPageData{
			PageBase: PageBase{
				Title:    "help.title",
				Locale:   locale,
				PageType: PageTypeHelp,
			},
			Endpoint:  helpPage.Endpoint,
			HelpLinks: s.help.sorted,
			Cname:     cnameVal,
		}); err != nil {
			s.deps.logger.Error().Err(err).Str("cname", cnameVal).Msg("failed to render help template")
		}
		w.Flush()
	})
}

// primaryLang extracts the primary language subtag from a BCP 47 tag.
// e.g. "zh-CN" -> "zh", "en" -> "en", "ja" -> "ja".
func primaryLang(tag string) string {
	if idx := strings.IndexByte(tag, '-'); idx != -1 {
		return tag[:idx]
	}
	return tag
}
