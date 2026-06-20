package index

import (
	"bytes"
	"context"
	"iter"
	"time"

	"github.com/bytedance/sonic"
	"github.com/openana/prism/pkg/meta"
	"github.com/rs/zerolog"
	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp"
)

var slashBytes = []byte("/")

type NginxFetcherConfig interface {
	FetcherConfig
	BaseURL() string // Should have trailing slash. e.g. "https://example.com/api/index/".
	Timeout() time.Duration
}

type NginxFetcher struct {
	cfg struct {
		baseURL string
	}

	deps struct {
		logger zerolog.Logger
		client *fasthttp.Client
	}
}

func NewNginxFetcher(cfg NginxFetcherConfig, logger zerolog.Logger) *NginxFetcher {
	p := &NginxFetcher{}

	p.cfg.baseURL = cfg.BaseURL()

	p.deps.logger = logger.With().Str("module", "index.NginxFetcher").Str("baseURL", p.cfg.baseURL).Logger()
	p.deps.client = &fasthttp.Client{
		ReadTimeout:         cfg.Timeout(),
		WriteTimeout:        cfg.Timeout(),
		MaxConnsPerHost:     512,
		MaxIdleConnDuration: 90 * time.Second,
	}

	return p
}

func (p *NginxFetcher) AllOrErr(ctx context.Context, path []byte) (iter.Seq[Entry], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	p.deps.logger.Debug().Bytes("path", path).Msg("fetching index")

	buf := bytebufferpool.Get()
	defer bytebufferpool.Put(buf)

	buf.WriteString(p.cfg.baseURL)
	buf.Write(bytes.TrimPrefix(path, slashBytes))
	if buf.B[buf.Len()-1] != '/' {
		buf.WriteByte('/')
	}

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	req.SetRequestURIBytes(buf.B)
	req.Header.SetMethod(fasthttp.MethodGet)
	req.Header.Set("User-Agent", meta.UserAgent)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(p.deps.client.ReadTimeout)
	}

	if err := p.deps.client.DoDeadline(req, resp, deadline); err != nil {
		p.deps.logger.Warn().Err(err).Bytes("path", path).Msg("http request failed")
		return nil, err
	}

	statusCode := resp.StatusCode()
	if statusCode != fasthttp.StatusOK {
		if statusCode == fasthttp.StatusForbidden || statusCode == fasthttp.StatusNotFound {
			p.deps.logger.Debug().Bytes("path", path).Int("status", statusCode).Msg("index not found")
			return nil, ErrNotFound
		} else {
			p.deps.logger.Warn().Bytes("path", path).Int("status", statusCode).Msg("unexpected upstream status")
			return nil, ErrUpstreamFailure
		}
	}

	var nes []NginxEntry

	if err := sonic.Unmarshal(resp.Body(), &nes); err != nil {
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

			mtime, err := time.Parse(time.RFC1123, ne.Mtime)
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
