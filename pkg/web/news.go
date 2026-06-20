package web

import (
	"bufio"
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
	"github.com/yuin/goldmark"
)

type NewsArticle struct {
	Title       string
	Date        string // YYYY-MM-DD
	Slug        string
	Description string
	BodyHTML    template.HTML
}

type NewsHeadline struct {
	Title       string
	Date        string
	Slug        string
	Description string
}

type NewsPageData struct {
	PageBase
	Article     *NewsArticle
	NewsLinks   []NewsHeadline
	CurrentSlug string
}

type newsFrontmatter struct {
	Title       string `yaml:"title"`
	Date        string `yaml:"date"`
	Slug        string `yaml:"slug"`
	Description string `yaml:"description"`
	Draft       bool   `yaml:"draft"`
}

func parseNewsFile(path string) (*NewsArticle, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("parseNewsFile: read: %w", err)
	}

	fm, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, fmt.Errorf("parseNewsFile: frontmatter: %w", err)
	}

	var fmData newsFrontmatter
	if err := yaml.Unmarshal(fm, &fmData); err != nil {
		return nil, fmt.Errorf("parseNewsFile: yaml: %w", err)
	}

	base := strings.TrimSuffix(filepath.Base(path), ".md")

	// fallback
	slug := fmData.Slug
	if slug == "" {
		slug = sanitizeSlug(base)
	}
	title := fmData.Title
	if title == "" {
		title = base
	}
	date := fmData.Date
	if date == "" {
		date = "1970-01-01"
	}

	var htmlBuf bytes.Buffer
	if err := goldmark.Convert(body, &htmlBuf); err != nil {
		return nil, fmt.Errorf("parseNewsFile: markdown: %w", err)
	}

	if fmData.Draft {
		return nil, nil // skip draft
	}

	return &NewsArticle{
		Title:       title,
		Date:        date,
		Slug:        slug,
		Description: fmData.Description,
		BodyHTML:    template.HTML(htmlBuf.String()),
	}, nil
}

func splitFrontmatter(data []byte) (fm []byte, body []byte, err error) {
	if !bytes.HasPrefix(data, []byte("---\n")) {
		return nil, data, nil // no frontmatter
	}

	rest := data[4:]
	end := bytes.Index(rest, []byte("\n---"))
	if end == -1 {
		return nil, data, nil
	}

	fm = rest[:end]
	body = rest[end+4:]
	if len(body) > 0 && body[0] == '\n' {
		body = body[1:]
	}
	return fm, body, nil
}

func sanitizeSlug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	result := b.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	return strings.Trim(result, "-")
}

func LoadNews(dir string, logger zerolog.Logger) (articles map[string]*NewsArticle, latest []NewsHeadline, sorted []NewsHeadline, err error) {
	if dir == "" {
		return nil, nil, nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warn().Str("dir", dir).Msg("news directory not found, skipping")
			return nil, nil, nil, nil
		}
		return nil, nil, nil, fmt.Errorf("loadNews: readdir: %w", err)
	}

	articles = make(map[string]*NewsArticle)
	var headlines []NewsHeadline
	now := time.Now().Format("2006-01-02")

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		fullPath := filepath.Join(dir, e.Name())
		a, err := parseNewsFile(fullPath)
		if err != nil {
			logger.Warn().Err(err).Str("file", e.Name()).Msg("failed to parse news file")
			continue
		}
		if a == nil {
			continue // draft
		}

		// Only include if date is not in the future.
		if a.Date > now {
			logger.Debug().Str("file", e.Name()).Str("date", a.Date).Msg("skipping future-dated news")
			continue
		}

		key := a.Date + "/" + a.Slug
		articles[key] = a

		headlines = append(headlines, NewsHeadline{
			Title:       a.Title,
			Date:        a.Date,
			Slug:        a.Slug,
			Description: a.Description,
		})
	}

	// Sort by date descending
	sort.Slice(headlines, func(i, j int) bool {
		return headlines[i].Date > headlines[j].Date
	})

	// for aside
	latest = headlines
	if len(latest) > 3 {
		latest = latest[:3]
	}

	sorted = headlines

	logger.Info().Int("count", len(headlines)).Str("dir", dir).Msg("loaded news articles")
	return articles, latest, sorted, nil
}

func (s *Server) HandleNews(ctx *fasthttp.RequestCtx) {
	dateVal, _ := ctx.UserValue("date").(string)
	slugVal, _ := ctx.UserValue("slug").(string)
	if dateVal == "" || slugVal == "" {
		s.handleNotFound(ctx, "/mirrors", "nav.mirrors")
		return
	}

	key := dateVal + "/" + slugVal
	article, ok := s.news.articles[key]
	if !ok {
		s.handleNotFound(ctx, "/mirrors", "nav.mirrors")
		return
	}

	locale := s.resolveLocale(ctx)
	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		if err := s.pages.news.ExecuteTemplate(w, "base", NewsPageData{
			PageBase: PageBase{
				Title:    "news.title",
				Locale:   locale,
				PageType: PageTypeNews,
				Nonce:    getUserValueString(ctx, "nonce"),
			},
			Article:     article,
			NewsLinks:   s.news.sorted,
			CurrentSlug: slugVal,
		}); err != nil {
			s.deps.logger.Error().Err(err).Msg("failed to render news template")
		}
		w.Flush()
	})
}

func (s *Server) HandleNewsLatest(ctx *fasthttp.RequestCtx) {
	if len(s.news.latest) == 0 {
		s.handleNotFound(ctx, "/mirrors", "nav.mirrors")
		return
	}
	latest := s.news.latest[0]
	ctx.Redirect("/news/"+latest.Date+"/"+latest.Slug, fasthttp.StatusFound)
}
