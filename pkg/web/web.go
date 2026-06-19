package web

import (
	"cmp"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/url"
	"slices"
	"strings"

	"github.com/openana/prism/pkg/index"
	"github.com/openana/prism/pkg/meta"
	"github.com/openana/prism/pkg/mirrors"
	"github.com/openana/prism/pkg/mirrors/cname"
	purl "github.com/openana/prism/pkg/url"
	"github.com/openana/prism/pkg/web/i18n"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
)

//go:embed templates/* static/*
var embeddedFS embed.FS

//go:embed templates/help/*
var helpEmbeddedFS embed.FS

var templateFS fs.FS
var staticFS fs.FS
var helpFS fs.FS

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
	helpFS, err = fs.Sub(helpEmbeddedFS, "templates/help")
	if err != nil {
		panic(err)
	}
}

// PageType identifies the kind of page being rendered, enabling type-specific
// behavior in shared templates (scripts, styles, menus, etc.).
type PageType uint8

const (
	PageTypeDefault PageType = iota
	PageTypeMirrors
	PageTypeStatus
	PageTypeDownloads
	PageTypeBrowse
	PageTypeHelp
	PageTypeNews
	PageNotFound
)

func (t PageType) IsDefault() bool   { return t == PageTypeDefault }
func (t PageType) IsMirrors() bool   { return t == PageTypeMirrors || t == PageTypeBrowse }
func (t PageType) IsStatus() bool    { return t == PageTypeStatus }
func (t PageType) IsDownloads() bool { return t == PageTypeDownloads }
func (t PageType) IsBrowse() bool    { return t == PageTypeBrowse }
func (t PageType) IsHelp() bool      { return t == PageTypeHelp }
func (t PageType) IsNews() bool      { return t == PageTypeNews }
func (t PageType) IsNotFound() bool  { return t == PageNotFound }

// PageBase holds fields common to all page data types passed to base.html.
type PageBase struct {
	Title    string
	Locale   *i18n.Locale
	PageType PageType
}

type Handler interface {
	HandleMirrors(ctx *fasthttp.RequestCtx)
	HandleDownloads(ctx *fasthttp.RequestCtx)
	HandleDownloadsDetail(ctx *fasthttp.RequestCtx)
	HandleStatus(ctx *fasthttp.RequestCtx)
	HandleStatic(ctx *fasthttp.RequestCtx)
	HandleBrowse(ctx *fasthttp.RequestCtx)
	HandleHelp(ctx *fasthttp.RequestCtx)
	HandleNews(ctx *fasthttp.RequestCtx)
	HandleNewsLatest(ctx *fasthttp.RequestCtx)
}

type Site struct {
	Name     string
	URL      string
	URLv4    string
	URLv6    string
	Homepage string
	Issues   string
	Request  string
	Email    string
	Group    string
	Disk     string
	Note     string
	Big      string
	Links    []LinkItem
}

type LinkItem struct {
	Name string
	URL  string
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

// HelpMirrorConfig describes a mirror that has a help page (auto or manual).
type HelpMirrorConfig struct {
	Name      string
	Mode      string
	URLPrefix string
	HelpURL   string
}

type ServerConfig interface {
	Site() Site
	ISOInfo() []ISOInfo
	HelpMirrors() []HelpMirrorConfig
	NewsDir() string
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
		help            map[string]*HelpPage // cname -> help page
		helpStart       *template.Template
		news            *template.Template
	}

	help struct {
		sorted []HelpLink
		m      map[string]HelpLink // name -> HelpLink
	}

	news struct {
		articles map[string]*NewsArticle // key: "date/slug"
		latest   []NewsHeadline          // top 3, for mirrors aside
		sorted   []NewsHeadline          // all published, for news page aside
	}
}

func NewServer(cfg ServerConfig, mirrorGetter mirrors.Getter, indexProvider index.Provider, pathResolver purl.Resolver, logger zerolog.Logger) (*Server, error) {
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
			return meta.VersionString
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

	// Parse help templates for mirrors with auto help mode.
	siteURL := cfg.Site().URL
	helpMirrors := cfg.HelpMirrors()
	s.pages.help = make(map[string]*HelpPage, len(helpMirrors))

	// Helper to parse a help page:
	parseHelpPage := func(helpFile string) *template.Template {
		tpl := template.New(helpFile).Funcs(funcMap)
		template.Must(tpl.ParseFS(templateFS, "base.html", "help_layout.html"))
		template.Must(tpl.ParseFS(helpFS, helpFile))
		return tpl
	}

	helps := make(map[string]HelpLink)

	entries, err := fs.ReadDir(helpFS, ".")
	if err != nil {
		return nil, fmt.Errorf("web.NewServer: failed to read templates: %w", err)
	}

	for _, hm := range helpMirrors {
		if hm.Mode == "auto" {
			cnameStr := cname.Cname(hm.Name)

			endpoint, err := url.JoinPath(siteURL, strings.TrimSuffix(hm.URLPrefix, "/"))
			if err != nil {
				return nil, fmt.Errorf("web.NewServer: failed to join endpoint: %w", err)
			}

			page := &HelpPage{
				ByLocale: make(map[string]*template.Template),
				Endpoint: endpoint,
			}

			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				prefix := cnameStr + "."
				if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ".html") {
					seenEn := false // Make sure en is the fallback if exists.
					// Extract lang: "alpine.zh.html" -> "zh"
					lang := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".html")
					tpl := parseHelpPage(name)
					page.ByLocale[lang] = tpl
					if !seenEn {
						page.Fallback = tpl
					}
					if lang == "en" {
						seenEn = true
					}
				}
			}

			if page.Fallback != nil {
				s.pages.help[cnameStr] = page
				helps[hm.Name] = HelpLink{
					Cname: cnameStr,
					URL:   "/help/" + cnameStr,
				}
			} else {
				s.deps.logger.Warn().Str("mirror", hm.Name).Str("cname", cnameStr).Msg("no built-in help for mirror")
			}

		} else if hm.Mode == "manual" {
			helps[hm.Name] = HelpLink{
				Cname: cname.Cname(hm.Name),
				URL:   hm.HelpURL,
			}
		}
	}

	// Add /help/start
	s.pages.helpStart = template.Must(template.New("help_start.html").Funcs(funcMap).ParseFS(
		templateFS,
		"base.html",
		"help_layout.html",
		"help_start.html",
	))

	sorted := make([]HelpLink, 0, len(helps))
	for _, h := range helps {
		sorted = append(sorted, h)
	}

	slices.SortFunc(sorted, func(a, b HelpLink) int {
		return cmp.Compare(a.Cname, b.Cname)
	})

	s.help.m = helps
	s.help.sorted = sorted

	// Parse news template.
	s.pages.news = parsePage("news.html")

	// Load news articles from filesystem.
	newsDir := cfg.NewsDir()
	articles, latest, sortedNews, err := loadNews(newsDir, s.deps.logger)
	if err != nil {
		return nil, fmt.Errorf("web.NewServer: %w", err)
	}
	s.news.articles = articles
	s.news.latest = latest
	s.news.sorted = sortedNews

	return s, nil
}

func (s *Server) resolveLocale(ctx *fasthttp.RequestCtx) *i18n.Locale {
	return i18n.Resolve(string(ctx.Request.Header.Peek("Accept-Language")))
}
