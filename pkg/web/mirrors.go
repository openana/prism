package web

import (
	"bufio"
	"strconv"
	"time"

	"github.com/openana/prism/pkg/mirrors"
	"github.com/valyala/fasthttp"
)

type Mirror struct {
	Name       string
	URL        string
	Desc       string
	Type       string
	Help       string
	LastUpdate string
}

func (s *Server) FormatMirrors(src *mirrors.Mirror) Mirror {
	tgt := Mirror{
		Name: src.Name,
	}

	if src.Metadata != nil {
		h, ok := s.help.m[src.Name]
		if ok {
			tgt.Help = h.URL
		}
		tgt.Type = src.Metadata.Type.String()
		tgt.Desc = src.Metadata.Desc

		switch src.Metadata.Type {
		case mirrors.Rsync:
			tgt.URL = "/browse?path=" + src.Metadata.URL
		case mirrors.Redirect:
			tgt.URL = src.Metadata.URL
		default:
		}
	}

	if src.Sync != nil {
		tgt.LastUpdate = time.Unix(src.Sync.LastUpdate, 0).UTC().Format(time.RFC3339)
	}

	return tgt
}

type MirrorPageData struct {
	PageBase
	Mirrors    []Mirror
	LatestNews []NewsHeadline
}

func (s *Server) HandleMirrors(ctx *fasthttp.RequestCtx) {
	it, age := s.deps.mirrorGetter.All()

	ctx.Response.Header.Set("Cache-Control", "public, max-age="+strconv.Itoa(int(s.deps.mirrorGetter.CacheTTL().Seconds())))
	ctx.Response.Header.Set("Age", strconv.Itoa(int(age.Seconds())))

	var mirrors []Mirror
	for m := range it {
		mirrors = append(mirrors, s.FormatMirrors(&m))
	}

	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		if err := s.pages.mirrors.ExecuteTemplate(w, "base", MirrorPageData{
			PageBase: PageBase{
				Title:    "mirrors.title",
				Locale:   s.resolveLocale(ctx),
				PageType: PageTypeMirrors,
			},
			Mirrors:    mirrors,
			LatestNews: s.news.latest,
		}); err != nil {
			s.deps.logger.Error().Err(err).Msg("failed to render template")
		}
		w.Flush()
	})
}
