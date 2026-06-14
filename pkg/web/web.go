package web

import (
	"bufio"
	"html/template"

	"github.com/openana/prism/pkg/index"
	"github.com/openana/prism/pkg/mirrors"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
)

type Handler interface {
	HandleMirrors(ctx *fasthttp.RequestCtx)
	HandleDownloads(ctx *fasthttp.RequestCtx)
}

type ServerConfig interface {
	Site() Site
}

type Server struct {
	cfg struct {
		site Site
	}

	deps struct {
		mirrorGetter  mirrors.Getter
		indexProvider index.Provider
		logger        zerolog.Logger
	}

	pages struct {
		mirrors *template.Template
	}
}

func NewServer(cfg ServerConfig, mirrorGetter mirrors.Getter, indexProvider index.Provider, logger zerolog.Logger) *Server {
	s := &Server{}

	s.cfg.site = cfg.Site()

	s.deps.mirrorGetter = mirrorGetter
	s.deps.indexProvider = indexProvider
	s.deps.logger = logger.With().Str("module", "web.Server").Logger()

	funcMap := template.FuncMap{
		"Site": func() *Site {
			return &s.cfg.site
		},
	}

	parsePage := func(pageFile string) *template.Template {
		return template.Must(template.New("").Funcs(funcMap).ParseFS(
			templateFS,
			"templates/base.html",
			"templates/"+pageFile,
		))
	}

	s.pages.mirrors = parsePage("mirrors.html")

	return s
}

func (s *Server) HandleMirrors(ctx *fasthttp.RequestCtx) {
	var mirrors []Mirror
	for m := range s.deps.mirrorGetter.All() {
		mirrors = append(mirrors, FormatMirrors(&m))
	}

	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		s.pages.mirrors.ExecuteTemplate(w, "base", MirrorPage{Mirrors: mirrors})
		w.Flush()
	})
}

func (s *Server) HandleDownloads(ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(fasthttp.StatusNotImplemented)
}
