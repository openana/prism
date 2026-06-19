package web

import (
	"bufio"
	"bytes"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/go-units"
	"github.com/openana/prism/pkg/index"
	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp"
)

// File/Directory in listing
type BrowseEntry struct {
	Name  string
	Size  string
	Mtime string
	Type  string
	URL   string
}

// single segment in the path breadcrumb.
type Breadcrumb struct {
	Label string
	URL   string
}

type BrowsePageData struct {
	PageBase
	Path        string
	Breadcrumbs []Breadcrumb
	Entries     []BrowseEntry
}

var slashBytes = []byte("/")

func (s *Server) HandleBrowse(ctx *fasthttp.RequestCtx) {
	locale := s.resolveLocale(ctx)

	rawPath := ctx.QueryArgs().Peek("path")
	if len(rawPath) == 0 {
		rawPath = slashBytes
	}
	if !bytes.HasPrefix(rawPath, slashBytes) {
		rawPath = append(slashBytes, rawPath...)
	}
	if !bytes.HasSuffix(rawPath, slashBytes) {
		rawPath = append(rawPath, '/')
	}

	// Resolve path
	pathBuf := bytebufferpool.Get()
	defer bytebufferpool.Put(pathBuf)

	resolvedPath, record, ok := s.deps.pathResolver.Append(rawPath, pathBuf.B)
	if !ok {
		s.handleNotFound(ctx, "/mirrors", "nav.mirrors")
		return
	}

	// Fetch from the index provider
	it, age, err := s.deps.indexProvider.AllOrErr(ctx, record.Host, resolvedPath)
	if err != nil {
		s.deps.logger.Warn().Err(err).Bytes("path", resolvedPath).Msg("index provider error")
		s.handleNotFound(ctx, "/mirrors", "nav.mirrors")
		return
	}

	ctx.Response.Header.Set("Cache-Control", "public, max-age="+strconv.Itoa(int(s.deps.indexProvider.CacheTTL().Seconds())))
	ctx.Response.Header.Set("Age", strconv.Itoa(int(age.Seconds())))

	// Collect and format entries.
	var entries []BrowseEntry
	for e := range it {
		be := BrowseEntry{
			Name:  e.Name,
			Mtime: time.Unix(e.Mtime, 0).UTC().Format(time.RFC3339),
			URL:   string(resolvedPath) + e.Name,
		}
		switch e.Type {
		case index.Directory:
			be.Type = "directory"
			be.URL = "/browse?path=" + be.URL + "/"
		case index.File:
			be.Type = "file"
			be.Size = units.HumanSize(float64(e.Size))
		default:
			be.Type = "other"
			be.Size = units.HumanSize(float64(e.Size))
		}
		entries = append(entries, be)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type == "directory"
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	breadcrumbs := buildBreadcrumbs(string(rawPath))

	page := BrowsePageData{
		PageBase: PageBase{
			Title:    "nav.browse",
			PageType: PageTypeBrowse,
			Locale:   locale,
		},
		Path:        string(rawPath),
		Breadcrumbs: breadcrumbs,
		Entries:     entries,
	}

	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		if err := s.pages.browse.ExecuteTemplate(w, "base", page); err != nil {
			s.deps.logger.Error().Err(err).Msg("failed to render browse template")
		}
		w.Flush()
	})
}

func buildBreadcrumbs(path string) []Breadcrumb {
	var crumbs []Breadcrumb

	if path == "/" {
		return crumbs
	}

	// Trim leading and trailing slashes for splitting
	trimmed := strings.TrimPrefix(path, "/")
	trimmed = strings.TrimSuffix(trimmed, "/")
	if trimmed == "" {
		return crumbs
	}

	parts := strings.Split(trimmed, "/")
	accum := "/"
	for _, part := range parts {
		accum += part + "/"
		crumbs = append(crumbs, Breadcrumb{
			Label: part,
			URL:   "/browse?path=" + accum,
		})
	}

	return crumbs
}
