// LLM usage: generated with deepseek-v4-pro and modified manually.
package router

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"iter"
	"strings"
	"testing"

	"github.com/openana/prism/pkg/index"
	"github.com/openana/prism/pkg/log"
	"github.com/openana/prism/pkg/mirrors"
	purl "github.com/openana/prism/pkg/url"
	"github.com/openana/prism/pkg/web"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
)

// --- Stub types for dependency injection ---

// stubRouterConfig implements RouterConfig.
type stubRouterConfig struct {
	protoHeader string
}

func (c stubRouterConfig) ProtoHeader() string { return c.protoHeader }

// stubResolver implements purl.Resolver.
type stubResolver struct {
	record purl.Record
	ok     bool
}

func (r stubResolver) Append(path []byte, dst []byte) ([]byte, purl.Record, bool) {
	return append(dst, path...), r.record, r.ok
}

// stubGetter implements mirrors.Getter.
type stubGetter struct {
	mirrors    []mirrors.Mirror
	mirrorz    *mirrors.Mirrorz
	mirrorzErr error
}

func (g stubGetter) All() iter.Seq[mirrors.Mirror] {
	return func(yield func(mirrors.Mirror) bool) {
		for _, m := range g.mirrors {
			if !yield(m) {
				return
			}
		}
	}
}

func (g stubGetter) Mirrorz() (*mirrors.Mirrorz, error) {
	return g.mirrorz, g.mirrorzErr
}

// stubProvider implements index.Provider.
type stubProvider struct {
	entries []index.Entry
	err     error
}

func (p stubProvider) AllOrErr(_ context.Context, _ string, _ []byte) (iter.Seq[index.Entry], error) {
	if p.err != nil {
		return nil, p.err
	}
	return func(yield func(index.Entry) bool) {
		for _, e := range p.entries {
			if !yield(e) {
				return
			}
		}
	}, nil
}

// stubWebHandler implements web.Handler.
type stubWebHandler struct {
	mirrorsStatus   int
	mirrorsBody     string
	downloadsStatus int
	downloadsBody   string
}

func (h stubWebHandler) HandleMirrors(ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(h.mirrorsStatus)
	ctx.SetBodyString(h.mirrorsBody)
}

func (h stubWebHandler) HandleDownloads(ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(h.downloadsStatus)
	ctx.SetBodyString(h.downloadsBody)
}

// --- Helpers ---

func newTestRouter(resolver purl.Resolver, getter mirrors.Getter, provider index.Provider, webHandler web.Handler) *Router {
	cfg := stubRouterConfig{protoHeader: "X-Forwarded-Proto"}
	discard := zerolog.New(io.Discard)
	return NewRouter(cfg, discard, log.AccessLogger(discard), resolver, getter, provider, webHandler)
}

func okResolver() stubResolver {
	return stubResolver{
		record: purl.Record{Host: "node1", FQDN: "mirrors.example.com"},
		ok:     true,
	}
}

func assertRedirect(t *testing.T, ctx *fasthttp.RequestCtx, wantScheme, wantHost, wantPath string) {
	t.Helper()
	if got := ctx.Response.StatusCode(); got != fasthttp.StatusMovedPermanently {
		t.Fatalf("expected status %d, got %d", fasthttp.StatusMovedPermanently, got)
	}
	want := wantScheme + "://" + wantHost + wantPath
	if got := string(ctx.Response.Header.Peek("Location")); got != want {
		t.Fatalf("expected Location %q, got %q", want, got)
	}
}

func assertNDJSONHeaders(t *testing.T, ctx *fasthttp.RequestCtx) {
	t.Helper()
	if ct := string(ctx.Response.Header.Peek("Content-Type")); ct != "application/x-ndjson" {
		t.Errorf("expected Content-Type application/x-ndjson, got %q", ct)
	}
	if te := string(ctx.Response.Header.Peek("Transfer-Encoding")); te != "chunked" {
		t.Errorf("expected Transfer-Encoding chunked, got %q", te)
	}
}

func assertNDJSONLines(t *testing.T, body []byte, wantLines int) [][]byte {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(body), []byte("\n"))
	if len(lines) != wantLines {
		t.Fatalf("expected %d JSON lines, got %d: %q", wantLines, len(lines), string(body))
	}
	return lines
}

func assertStatusAndBodyContains(t *testing.T, ctx *fasthttp.RequestCtx, wantStatus int, wantSubstr string) {
	t.Helper()
	if got := ctx.Response.StatusCode(); got != wantStatus {
		t.Fatalf("expected status %d, got %d", wantStatus, got)
	}
	if !strings.Contains(string(ctx.Response.Body()), wantSubstr) {
		t.Errorf("expected body to contain %q, got %q", wantSubstr, string(ctx.Response.Body()))
	}
}

// --- Phase 1: Tracer bullet — Redirect happy path ---

func TestRouter_Redirect_ResolverOK(t *testing.T) {
	router := newTestRouter(okResolver(), stubGetter{}, stubProvider{}, stubWebHandler{})

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/ubuntu/pool/main/x86_64/somefile.rpm")
	ctx.Request.Header.Set("X-Forwarded-Proto", "http")

	router.HandleRequest(&ctx)

	assertRedirect(t, &ctx, "http", "mirrors.example.com", "/ubuntu/pool/main/x86_64/somefile.rpm")
}

// --- Phase 2: Index streaming ---

func TestRouter_Index_Success(t *testing.T) {
	provider := stubProvider{
		entries: []index.Entry{
			{Name: "file1.txt", Size: 100, Mtime: 1700000000, Type: index.File},
			{Name: "dir1", Size: 0, Mtime: 1700000001, Type: index.Directory},
		},
	}
	router := newTestRouter(okResolver(), stubGetter{}, provider, stubWebHandler{})

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/api/index?path=%2Falpine%2F")

	router.HandleRequest(&ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected status %d, got %d", fasthttp.StatusOK, got)
	}
	assertNDJSONHeaders(t, &ctx)

	lines := assertNDJSONLines(t, ctx.Response.Body(), 2)
	for i, line := range lines {
		var entry index.Entry
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Errorf("line %d is not valid JSON: %q: %v", i, string(line), err)
		}
	}
}

func TestRouter_Index_Errors(t *testing.T) {
	tests := []struct {
		name       string
		resolver   stubResolver
		err        error
		wantStatus int
		wantMsg    string
	}{
		{
			name:       "ResolverFails",
			resolver:   stubResolver{ok: false},
			err:        nil,
			wantStatus: fasthttp.StatusNotFound,
			wantMsg:    "path not resolved",
		},
		{
			name:       "ProviderNotFound",
			resolver:   okResolver(),
			err:        index.ErrNotFound,
			wantStatus: fasthttp.StatusNotFound,
			wantMsg:    "index not found",
		},
		{
			name:       "ProviderUpstreamFailure",
			resolver:   okResolver(),
			err:        index.ErrUpstreamFailure,
			wantStatus: fasthttp.StatusBadGateway,
			wantMsg:    "upstream failure",
		},
		{
			name:       "ProviderOtherError",
			resolver:   okResolver(),
			err:        io.ErrUnexpectedEOF,
			wantStatus: fasthttp.StatusInternalServerError,
			wantMsg:    "internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := stubProvider{err: tt.err}
			router := newTestRouter(tt.resolver, stubGetter{}, provider, stubWebHandler{})

			var ctx fasthttp.RequestCtx
			ctx.Request.SetRequestURI("/api/index?path=%2Falpine%2F")

			router.HandleRequest(&ctx)

			assertStatusAndBodyContains(t, &ctx, tt.wantStatus, tt.wantMsg)
		})
	}
}

// --- Phase 3: Mirrors & Mirrorz ---

func TestRouter_Mirrors_Success(t *testing.T) {
	getter := stubGetter{
		mirrors: []mirrors.Mirror{
			{Name: "alpine", Metadata: &mirrors.Metadata{Desc: "Alpine Linux"}},
			{Name: "ubuntu", Metadata: &mirrors.Metadata{Desc: "Ubuntu Linux"}},
		},
	}
	router := newTestRouter(stubResolver{ok: false}, getter, stubProvider{}, stubWebHandler{})

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/api/mirrors")

	router.HandleRequest(&ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected status %d, got %d", fasthttp.StatusOK, got)
	}
	assertNDJSONHeaders(t, &ctx)

	lines := assertNDJSONLines(t, ctx.Response.Body(), 2)
	for i, line := range lines {
		var m mirrors.Mirror
		if err := json.Unmarshal(line, &m); err != nil {
			t.Errorf("line %d is not valid JSON: %q: %v", i, string(line), err)
		}
	}
}

func TestRouter_Mirrors_Empty(t *testing.T) {
	getter := stubGetter{}
	router := newTestRouter(stubResolver{ok: false}, getter, stubProvider{}, stubWebHandler{})

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/api/mirrors")

	router.HandleRequest(&ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected status %d, got %d", fasthttp.StatusOK, got)
	}

	body := ctx.Response.Body()
	if len(body) > 0 && !bytes.Equal(body, []byte{}) {
		// May be empty or have just whitespace since iterator yielded nothing
		if len(bytes.TrimSpace(body)) != 0 {
			t.Errorf("expected empty body, got %q", string(body))
		}
	}
}

func TestRouter_Mirrorz_Success(t *testing.T) {
	mz := &mirrors.Mirrorz{
		Version: mirrors.MzVersion,
		Site: mirrors.Site{
			URL:  "https://example.org",
			Abbr: "EXAMPLE",
		},
		Mirrors: []mirrors.MirrorzEntry{
			{Cname: "alpine", Desc: "Alpine Linux"},
		},
	}
	getter := stubGetter{mirrorz: mz}
	router := newTestRouter(stubResolver{ok: false}, getter, stubProvider{}, stubWebHandler{})

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/api/mirrorz")
	ctx.Request.Header.SetMethod("GET")

	router.HandleRequest(&ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected status %d, got %d", fasthttp.StatusOK, got)
	}
	if ct := string(ctx.Response.Header.Peek("Content-Type")); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var decoded mirrors.Mirrorz
	if err := json.Unmarshal(ctx.Response.Body(), &decoded); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
	if len(decoded.Mirrors) != 1 || decoded.Mirrors[0].Cname != "alpine" {
		t.Errorf("unexpected mirrors content: %+v", decoded.Mirrors)
	}
}

func TestRouter_Mirrorz_HEAD(t *testing.T) {
	getter := stubGetter{
		mirrorz:    nil, // would panic if Mirrorz() were called
		mirrorzErr: nil,
	}
	router := newTestRouter(stubResolver{ok: false}, getter, stubProvider{}, stubWebHandler{})

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/api/mirrorz")
	ctx.Request.Header.SetMethod("HEAD")

	router.HandleRequest(&ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected status %d, got %d", fasthttp.StatusOK, got)
	}
	if len(ctx.Response.Body()) != 0 {
		t.Errorf("expected empty body for HEAD request, got %q", string(ctx.Response.Body()))
	}
}

func TestRouter_Mirrorz_Error(t *testing.T) {
	getter := stubGetter{mirrorzErr: io.ErrUnexpectedEOF}
	router := newTestRouter(stubResolver{ok: false}, getter, stubProvider{}, stubWebHandler{})

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/api/mirrorz")
	ctx.Request.Header.SetMethod("GET")

	router.HandleRequest(&ctx)

	assertStatusAndBodyContains(t, &ctx, fasthttp.StatusInternalServerError, "internal server error")
}

func TestRouter_Mirrorz_Empty(t *testing.T) {
	getter := stubGetter{mirrorz: &mirrors.Mirrorz{
		Site: mirrors.Site{URL: "https://example.org", Abbr: "EX"},
	}}
	router := newTestRouter(stubResolver{ok: false}, getter, stubProvider{}, stubWebHandler{})

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/api/mirrorz")

	router.HandleRequest(&ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected status %d, got %d", fasthttp.StatusOK, got)
	}
	var decoded mirrors.Mirrorz
	if err := json.Unmarshal(ctx.Response.Body(), &decoded); err != nil {
		t.Fatalf("response body is not valid JSON: %v", err)
	}
}

// --- Phase 4: Pages routes ---

func TestRouter_Pages_Mirrors_Success(t *testing.T) {
	wh := stubWebHandler{
		mirrorsStatus: fasthttp.StatusOK,
		mirrorsBody:   "<html>mirrors page</html>",
	}
	router := newTestRouter(stubResolver{ok: false}, stubGetter{}, stubProvider{}, wh)

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/pages/mirrors")

	router.HandleRequest(&ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusOK {
		t.Fatalf("expected status %d, got %d", fasthttp.StatusOK, got)
	}
	if got := string(ctx.Response.Body()); got != "<html>mirrors page</html>" {
		t.Errorf("expected mirrors page body, got %q", got)
	}
}

func TestRouter_Pages_Downloads(t *testing.T) {
	wh := stubWebHandler{
		downloadsStatus: fasthttp.StatusNotImplemented,
		downloadsBody:   "",
	}
	router := newTestRouter(stubResolver{ok: false}, stubGetter{}, stubProvider{}, wh)

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/pages/downloads")

	router.HandleRequest(&ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusNotImplemented {
		t.Fatalf("expected status %d, got %d", fasthttp.StatusNotImplemented, got)
	}
}

func TestRouter_Pages_NotFound(t *testing.T) {
	router := newTestRouter(stubResolver{ok: false}, stubGetter{}, stubProvider{}, stubWebHandler{})

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/pages/nonexistent")

	router.HandleRequest(&ctx)

	assertStatusAndBodyContains(t, &ctx, fasthttp.StatusNotFound, "page not found")
}

// --- Phase 5: Error paths ---

// TestRouter_RootPath_RedirectFails verifies that requesting "/" falls through
// to the redirect handler, which fails when the resolver has no match.
func TestRouter_RootPath_RedirectFails(t *testing.T) {
	router := newTestRouter(stubResolver{ok: false}, stubGetter{}, stubProvider{}, stubWebHandler{})

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/")

	router.HandleRequest(&ctx)

	// "/" strips to "" which doesn't match api/ or static/, so it goes to redirect.
	assertStatusAndBodyContains(t, &ctx, fasthttp.StatusNotFound, "path not resolved")
}

func TestRouter_InvalidAPI(t *testing.T) {
	router := newTestRouter(stubResolver{ok: false}, stubGetter{}, stubProvider{}, stubWebHandler{})

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/api/nonexistent")

	router.HandleRequest(&ctx)

	assertStatusAndBodyContains(t, &ctx, fasthttp.StatusNotFound, "not found")
}

// --- Phase 5: Proto sniffing & edge cases ---

func TestRouter_Redirect_ProtoSniffing(t *testing.T) {
	tests := []struct {
		name       string
		proto      string
		wantScheme string
	}{
		{"HTTPS", "https", "https"},
		{"HTTP", "http", "http"},
	}

	path := "/ubuntu/pool/main/x86_64/somefile.rpm"

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newTestRouter(okResolver(), stubGetter{}, stubProvider{}, stubWebHandler{})

			var ctx fasthttp.RequestCtx
			ctx.Request.SetRequestURI(path)
			ctx.Request.Header.Set("X-Forwarded-Proto", tt.proto)

			router.HandleRequest(&ctx)

			assertRedirect(t, &ctx, tt.wantScheme, "mirrors.example.com", path)
		})
	}
}

func TestRouter_Static(t *testing.T) {
	router := newTestRouter(stubResolver{ok: false}, stubGetter{}, stubProvider{}, stubWebHandler{})

	var ctx fasthttp.RequestCtx
	ctx.Request.SetRequestURI("/static/app.js")

	router.HandleRequest(&ctx)

	if got := ctx.Response.StatusCode(); got != fasthttp.StatusNotImplemented {
		t.Fatalf("expected status %d, got %d", fasthttp.StatusNotImplemented, got)
	}
}
