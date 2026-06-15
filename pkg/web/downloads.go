package web

import (
	"bufio"
	"net/url"

	"github.com/openana/prism/pkg/web/i18n"
	"github.com/valyala/fasthttp"
)

type CategoryGroup struct {
	Category string
	Distros  []string
}

type DownloadsIndexPage struct {
	Locale     *i18n.Locale
	Categories []CategoryGroup
}

type DownloadsDetailPage struct {
	Locale *i18n.Locale
	Info   ISOInfo
}

func GroupByCategory(infos []ISOInfo) []CategoryGroup {
	if len(infos) == 0 {
		return nil
	}

	// preserve order
	var groups []CategoryGroup
	seen := map[string]int{} // category -> index

	for _, info := range infos {
		idx, ok := seen[info.Category]
		if !ok {
			idx = len(groups)
			seen[info.Category] = idx
			groups = append(groups, CategoryGroup{Category: info.Category})
		}
		groups[idx].Distros = append(groups[idx].Distros, info.Distro)
	}

	return groups
}

func (s *Server) HandleDownloads(ctx *fasthttp.RequestCtx) {
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		page := DownloadsIndexPage{
			Locale:     s.resolveLocale(ctx),
			Categories: s.cfg.categories,
		}
		if err := s.pages.downloads.ExecuteTemplate(w, "base", page); err != nil {
			s.deps.logger.Error().Err(err).Msg("failed to render template")
		}
		w.Flush()
	})
}

func (s *Server) HandleDownloadsDetail(ctx *fasthttp.RequestCtx) {
	distro, ok := ctx.UserValue("distro").(string)
	if !ok || distro == "" {
		s.HandleNotFound(ctx)
		return
	}

	var err error
	distro, err = url.PathUnescape(distro)
	if err != nil {
		s.HandleNotFound(ctx)
		return
	}

	idx, ok := s.cfg.isoInfoIdx[distro]
	if !ok {
		s.HandleNotFound(ctx)
		return
	}

	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		if err := s.pages.downloadsDetail.ExecuteTemplate(w, "base", DownloadsDetailPage{Locale: s.resolveLocale(ctx), Info: s.cfg.isoInfo[idx]}); err != nil {
			s.deps.logger.Error().Err(err).Msg("failed to render template")
		}
		w.Flush()
	})
}

func categoryCname(s string) string {
	switch s {
	case "os":
		return "OS"
	case "app":
		return "Application"
	case "font":
		return "Font"
	default:
		return s
	}
}
