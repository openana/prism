package web

import (
	"bufio"
	"net/url"

	"github.com/valyala/fasthttp"
)

type CategoryGroup struct {
	Category string
	Distros  []string
}

type DownloadsIndexPageData struct {
	PageBase
	Categories []CategoryGroup
}

type DownloadsDetailPageData struct {
	PageBase
	Info ISOInfo
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
	ctx.Response.Header.Set("Cache-Control", "public, max-age=3600")

	nonce := getUserValueString(ctx, "nonce")

	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		page := DownloadsIndexPageData{
			PageBase: PageBase{
				Locale:   s.resolveLocale(ctx),
				PageType: PageTypeDownloads,
				Title:    "downloads.title",
				Nonce:    nonce,
			},
			Categories: s.cfg.categories,
		}
		if err := s.pages.downloads.ExecuteTemplate(w, "base", page); err != nil {
			s.deps.logger.Error().Err(err).Msg("failed to render template")
		}
		w.Flush()
	})
}

func (s *Server) HandleDownloadsDetail(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Cache-Control", "public, max-age=3600")

	distro, ok := ctx.UserValue("distro").(string)
	if !ok || distro == "" {
		s.handleNotFound(ctx, "/downloads", "nav.downloads")
		return
	}

	var err error
	distro, err = url.PathUnescape(distro)
	if err != nil {
		s.handleNotFound(ctx, "/downloads", "nav.downloads")
		return
	}

	idx, ok := s.cfg.isoInfoIdx[distro]
	if !ok {
		s.handleNotFound(ctx, "/downloads", "nav.downloads")
		return
	}

	nonce := getUserValueString(ctx, "nonce")

	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		if err := s.pages.downloadsDetail.ExecuteTemplate(w, "base", DownloadsDetailPageData{PageBase: PageBase{
			Locale:   s.resolveLocale(ctx),
			PageType: PageTypeDownloads,
			Title:    "downloads.title",
			Nonce:    nonce,
		}, Info: s.cfg.isoInfo[idx]}); err != nil {
			s.deps.logger.Error().Err(err).Msg("failed to render template")
		}
		w.Flush()
	})
}

func categoryAlias(s string) string {
	switch s {
	case "os":
		return "downloads.category_alias.os"
	case "app":
		return "downloads.category_alias.app"
	case "font":
		return "downloads.category_alias.font"
	default:
		return s
	}
}
