package web

import (
	"bufio"

	"github.com/valyala/fasthttp"
)

// NotFoundPageData is the data passed to the 404 template.
type NotFoundPageData struct {
	PageBase
	BackURL   string
	BackTitle string
}

func (s *Server) handleNotFound(ctx *fasthttp.RequestCtx, backURL string, backTitle string) {
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetStatusCode(fasthttp.StatusNotFound)
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		if err := s.pages.notFound.ExecuteTemplate(w, "base", NotFoundPageData{
			PageBase: PageBase{
				Title:    "error.title",
				Locale:   s.resolveLocale(ctx),
				PageType: PageNotFound,
			},
			BackURL:   backURL,
			BackTitle: backTitle,
		}); err != nil {
			s.deps.logger.Error().Err(err).Msg("failed to render 404 template")
		}
		w.Flush()
	})
}
