package web

import (
	"embed"
	"html/template"
	"io/fs"

	"github.com/openana/prism/pkg/index"
	"github.com/openana/prism/pkg/meta"
	"github.com/openana/prism/pkg/mirrors"
	purl "github.com/openana/prism/pkg/url"
	"github.com/openana/prism/pkg/web/i18n"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
)

//go:embed templates/* static/*
var embeddedFS embed.FS
var templateFS fs.FS
var staticFS fs.FS

func init() {
	var err error
	templateFS, err = fs.Sub(embeddedFS, "templates")
	if err != nil {
		panic(err)
	}
	staticFS, err = fs.Sub(embeddedFS, "static")
	if err != nil {
		panic(err)
	}
}

type Handler interface {
	HandleMirrors(ctx *fasthttp.RequestCtx)
	HandleDownloads(ctx *fasthttp.RequestCtx)
	HandleDownloadsDetail(ctx *fasthttp.RequestCtx)
	HandleStatus(ctx *fasthttp.RequestCtx)
	HandleStatic(ctx *fasthttp.RequestCtx)
	HandleBrowse(ctx *fasthttp.RequestCtx)
	HandleNotFound(ctx *fasthttp.RequestCtx)
}

type Site struct {
	Name     string
	URL      string
	Homepage string
	Issues   string
	Request  string
	Email    string
	Group    string
	Disk     string
	Note     string
	Big      string
}

type ISODownload struct {
	Name string
	URL  string
}

type ISOInfo struct {
	Distro   string
	Category string
	URLs     []ISODownload
}

type ServerConfig interface {
	Site() Site
	ISOInfo() []ISOInfo
}

type Server struct {
	cfg struct {
		site       Site
		isoInfo    []ISOInfo
		isoInfoIdx map[string]int
		categories []CategoryGroup
	}

	deps struct {
		mirrorGetter  mirrors.Getter
		indexProvider index.Provider
		pathResolver  purl.Resolver
		logger        zerolog.Logger
	}

	pages struct {
		mirrors         *template.Template
		status          *template.Template
		downloads       *template.Template
		downloadsDetail *template.Template
		browse          *template.Template
		notFound        *template.Template
	}
}

func NewServer(cfg ServerConfig, mirrorGetter mirrors.Getter, indexProvider index.Provider, pathResolver purl.Resolver, logger zerolog.Logger) *Server {
	s := &Server{}

	s.cfg.site = cfg.Site()
	s.cfg.isoInfo = cfg.ISOInfo()
	s.cfg.isoInfoIdx = make(map[string]int, len(s.cfg.isoInfo))
	for i, info := range s.cfg.isoInfo {
		s.cfg.isoInfoIdx[info.Distro] = i
	}
	s.cfg.categories = GroupByCategory(s.cfg.isoInfo)

	s.deps.mirrorGetter = mirrorGetter
	s.deps.indexProvider = indexProvider
	s.deps.pathResolver = pathResolver
	s.deps.logger = logger.With().Str("module", "web.Server").Logger()

	funcMap := template.FuncMap{
		"site": func() *Site {
			return &s.cfg.site
		},
		"version": func() string {
			return meta.ServerName
		},
		"catAlias": categoryAlias,
	}

	parsePage := func(pageFile string) *template.Template {
		return template.Must(template.New(pageFile).Funcs(funcMap).ParseFS(
			templateFS,
			"base.html",
			pageFile,
		))
	}

	s.pages.mirrors = parsePage("mirrors.html")
	s.pages.status = parsePage("status.html")
	s.pages.downloads = parsePage("downloads.html")
	s.pages.downloadsDetail = parsePage("downloads_detail.html")
	s.pages.browse = parsePage("browse.html")
	s.pages.notFound = parsePage("404.html")

	return s
}

func (s *Server) resolveLocale(ctx *fasthttp.RequestCtx) *i18n.Locale {
	return i18n.Resolve(string(ctx.Request.Header.Peek("Accept-Language")))
}
