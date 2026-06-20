package web

import (
	"bufio"
	"bytes"
	"fmt"
	"html/template"
	"os"

	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
	"github.com/yuin/goldmark"
)

// Return empty if path is empty
func LoadAbout(filePath string, logger zerolog.Logger) (template.HTML, error) {
	if filePath == "" {
		return "", nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("LoadAbout: read: %w", err)
	}

	var htmlBuf bytes.Buffer
	if err := goldmark.Convert(data, &htmlBuf); err != nil {
		return "", fmt.Errorf("LoadAbout: markdown: %w", err)
	}

	logger.Info().Str("file", filePath).Msg("loaded about page")
	return template.HTML(htmlBuf.String()), nil
}

type AboutPageData struct {
	PageBase
	BodyHTML template.HTML
}

func (s *Server) HandleAbout(ctx *fasthttp.RequestCtx) {
	locale := s.resolveLocale(ctx)
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		if err := s.pages.about.ExecuteTemplate(w, "base", AboutPageData{
			PageBase: PageBase{
				Title:    "about.title",
				Locale:   locale,
				PageType: PageTypeAbout,
			},
			BodyHTML: s.aboutHTML,
		}); err != nil {
			s.deps.logger.Error().Err(err).Msg("failed to render about template")
		}
	})
}
