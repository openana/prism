package web

import (
	"bufio"
	"time"

	"github.com/openana/prism/pkg/mirrors"
	"github.com/openana/prism/pkg/web/i18n"
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

func FormatMirrors(src *mirrors.Mirror) Mirror {
	tgt := Mirror{
		Name: src.Name,
	}

	if src.Metadata != nil {
		tgt.Help = src.Metadata.HelpURL
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

type MirrorPage struct {
	Locale  *i18n.Locale
	Mirrors []Mirror
}

func (s *Server) HandleMirrors(ctx *fasthttp.RequestCtx) {
	var mirrors []Mirror
	for m := range s.deps.mirrorGetter.All() {
		mirrors = append(mirrors, FormatMirrors(&m))
	}

	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		if err := s.pages.mirrors.ExecuteTemplate(w, "base", MirrorPage{Locale: s.resolveLocale(ctx), Mirrors: mirrors}); err != nil {
			s.deps.logger.Error().Err(err).Msg("failed to render template")
		}
		w.Flush()
	})
}
