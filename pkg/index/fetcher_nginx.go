package index

import (
	"bytes"
	"context"
	"encoding/json"
	"iter"
	"net/http"
	"time"

	"github.com/openana/prism/pkg/meta"
	"github.com/rs/zerolog"
	"github.com/valyala/bytebufferpool"
)

var slashBytes = []byte("/")

type NginxFetcherConfig interface {
	BaseURL() string // Should have trailing slash. e.g. "https://example.com/api/index/".
	Timeout() time.Duration
	TimeLayout() string
}

type NginxFetcher struct {
	cfg struct {
		baseURL    string
		timeout    time.Duration
		timeLayout string
	}

	deps struct {
		logger zerolog.Logger
	}
}

func NewNginxFetcher(cfg NginxFetcherConfig, logger zerolog.Logger) *NginxFetcher {
	p := &NginxFetcher{}

	p.cfg.baseURL = cfg.BaseURL()
	p.cfg.timeout = cfg.Timeout()
	p.cfg.timeLayout = cfg.TimeLayout()

	p.deps.logger = logger.With().Str("module", "NginxFetcher").Str("baseURL", p.cfg.baseURL).Logger()

	return p
}

func (p *NginxFetcher) AllOrErr(ctx context.Context, path []byte) (iter.Seq[Entry], error) {
	buf := bytebufferpool.Get()
	defer bytebufferpool.Put(buf)

	buf.WriteString(p.cfg.baseURL)
	buf.Write(bytes.TrimPrefix(path, slashBytes))
	if buf.B[buf.Len()-1] != '/' {
		buf.WriteByte('/')
	}

	ctx, cancel := context.WithTimeout(ctx, p.cfg.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, buf.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", meta.UserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusNotFound {
			return nil, ErrNotFound
		} else {
			return nil, ErrUpstreamFailure
		}
	}

	var nes []NginxEntry

	if err := json.NewDecoder(resp.Body).Decode(&nes); err != nil {
		return nil, err
	}

	return func(yield func(Entry) bool) {
		for _, ne := range nes {
			var t EntryType
			switch ne.Type {
			case "directory":
				t = Directory
			case "file":
				t = File
			default:
				fallthrough
			case "other":
				t = Other
			}

			mtime, err := time.Parse(p.cfg.timeLayout, ne.Mtime)
			if err != nil {
				p.deps.logger.Warn().Err(err).Str("mtime", ne.Mtime).Msg("parse mtime failed")
				continue
			}

			if !yield(Entry{
				Name:  ne.Name,
				Size:  ne.Size,
				Mtime: mtime.Unix(),
				Type:  t,
			}) {
				return
			}
		}
	}, nil
}

type NginxEntry struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Mtime string `json:"mtime"`
	Size  int64  `json:"size"`
}
