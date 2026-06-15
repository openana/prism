package web

import (
	"bufio"

	"github.com/openana/prism/pkg/web/i18n"
	"github.com/valyala/fasthttp"
)

// NotFoundPage is the data passed to the 404 template.
type NotFoundPage struct {
	Locale *i18n.Locale
}

func (s *Server) HandleNotFound(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetStatusCode(fasthttp.StatusNotFound)
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		if err := s.pages.notFound.ExecuteTemplate(w, "base", NotFoundPage{
			Locale: s.resolveLocale(ctx),
		}); err != nil {
			s.deps.logger.Error().Err(err).Msg("failed to render 404 template")
		}
		w.Flush()
	})
}
