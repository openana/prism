package router

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/bytedance/sonic"
	"github.com/fasthttp/router"
	"github.com/openana/prism/pkg/index"
	"github.com/openana/prism/pkg/log"
	"github.com/openana/prism/pkg/mirrors"
	"github.com/openana/prism/pkg/mirrorz"
	purl "github.com/openana/prism/pkg/url"
	"github.com/openana/prism/pkg/web"
	"github.com/rs/zerolog"
	"github.com/valyala/bytebufferpool"
	"github.com/valyala/fasthttp"
)

type Handler interface {
	HandleRequest(ctx *fasthttp.RequestCtx)
}

type RouterConfig interface {
	ProtoHeader() string
	RemoteIPHeader() string
	SiteURL() string
	SiteURLv4() string
	SiteURLv6() string
}

// Router is the top level fasthttp handler provider.
type Router struct {
	cfg struct {
		protoHeader    string
		remoteIPHeader string
		siteURL        []byte
		siteURLv4      []byte
		siteURLv6      []byte
	}

	deps struct {
		logger          zerolog.Logger
		accessLogger    zerolog.Logger
		pathResolver    purl.Resolver
		mirrorGetter    mirrors.Getter
		mirrorzProvider mirrorz.Provider
		indexProvider   index.Provider
		webHandler      web.Handler
	}

	r *router.Router
}

func NewRouter(cfg RouterConfig, logger zerolog.Logger, accessLogger log.AccessLogger, pathResolver purl.Resolver, mirrorGetter mirrors.Getter, mirrorzProvider mirrorz.Provider, indexProvider index.Provider, webHandler web.Handler) *Router {
	rt := &Router{}

	rt.cfg.protoHeader = cfg.ProtoHeader()
	rt.cfg.remoteIPHeader = cfg.RemoteIPHeader()
	rt.cfg.siteURL = []byte(cfg.SiteURL())
	rt.cfg.siteURLv4 = []byte(cfg.SiteURLv4())
	rt.cfg.siteURLv6 = []byte(cfg.SiteURLv6())

	rt.deps.logger = logger.With().Str("module", "router.Router").Logger()
	rt.deps.accessLogger = zerolog.Logger(accessLogger)
	rt.deps.pathResolver = pathResolver
	rt.deps.mirrorGetter = mirrorGetter
	rt.deps.mirrorzProvider = mirrorzProvider
	rt.deps.indexProvider = indexProvider
	rt.deps.webHandler = webHandler

	r := router.New()

	// Register routes

	// Page routes
	r.GET("/", rt.handleRootRedirect)
	r.GET("/status", rt.deps.webHandler.HandleStatus)
	r.GET("/mirrors", rt.deps.webHandler.HandleMirrors)
	r.GET("/browse", rt.deps.webHandler.HandleBrowse)
	r.GET("/downloads", rt.deps.webHandler.HandleDownloads)
	r.GET("/downloads/{distro}", rt.deps.webHandler.HandleDownloadsDetail)
	r.GET("/help", rt.deps.webHandler.HandleHelpIndex)
	r.GET("/help/{cname}", rt.deps.webHandler.HandleHelp)
	r.GET("/news/latest", rt.deps.webHandler.HandleNewsLatest)
	r.GET("/news/{date}/{slug}", rt.deps.webHandler.HandleNews)
	r.GET("/about", rt.deps.webHandler.HandleAbout)
	r.GET("/banme", rt.handleBanMe)

	// API routes
	r.GET("/api/ping", rt.handlePing)
	r.GET("/api/index", rt.handleIndex)
	r.GET("/api/mirrors", rt.handleMirrorsRequest)
	r.GET("/api/mirrorz", rt.handleMirrorzRequest)
	r.HEAD("/api/mirrorz", rt.handleMirrorzHead)

	// Static assets
	r.ANY("/static/{path:*}", rt.deps.webHandler.HandleStatic)

	// Redirect
	r.ANY("/{path:*}", rt.handleRedirect)

	rt.r = r

	return rt
}

var (
	protoHTTPBytes  = []byte("http")
	protoHTTPSBytes = []byte("https")

	cspHeaderTemplate = []byte("default-src 'self'; script-src 'self' 'nonce-")
	cspStylePart      = []byte("'; style-src 'self' 'nonce-")

	nonceKey              = "nonce"
	xFrameOptionsValue    = []byte("DENY")
	xContentTypeOptsValue = []byte("nosniff")
	referrerPolicyValue   = []byte("strict-origin-when-cross-origin")
)

func (rt *Router) HandleRequest(ctx *fasthttp.RequestCtx) {
	// Generate per-request CSP nonce
	var nonceBytes [16]byte
	if _, err := rand.Read(nonceBytes[:]); err != nil {
		rt.deps.logger.Error().Err(err).Msg("failed to generate nonce")
	}
	nonceHex := make([]byte, hex.EncodedLen(len(nonceBytes)))
	hex.Encode(nonceHex, nonceBytes[:])

	// Inject security headers
	ctx.Response.Header.SetBytesV("X-Frame-Options", xFrameOptionsValue)
	ctx.Response.Header.SetBytesV("X-Content-Type-Options", xContentTypeOptsValue)
	ctx.Response.Header.SetBytesV("Referrer-Policy", referrerPolicyValue)

	// Build CSP: "default-src 'self'; script-src 'self' 'nonce-<hex>'; style-src 'self' 'nonce-<hex>'"
	cspBuf := bytebufferpool.Get()
	defer bytebufferpool.Put(cspBuf)
	cspBuf.B = append(cspBuf.B, cspHeaderTemplate...)
	cspBuf.B = append(cspBuf.B, nonceHex...)
	cspBuf.B = append(cspBuf.B, cspStylePart...)
	cspBuf.B = append(cspBuf.B, nonceHex...)
	cspBuf.B = append(cspBuf.B, '\'')
	ctx.Response.Header.SetBytesV("Content-Security-Policy", cspBuf.B)

	// Store nonce
	ctx.SetUserValue(nonceKey, nonceHex)

	rt.r.Handler(ctx)

	ip := ctx.Request.Header.Peek(rt.cfg.remoteIPHeader)
	if len(ip) != 0 {
		idx := bytes.IndexByte(ip, ',')
		if idx >= 0 {
			ip = ip[:idx]
		}
	}

	rt.deps.accessLogger.Info().
		Int("status", ctx.Response.StatusCode()).
		Bytes("remote_ip", ip).
		Bytes("uri", ctx.URI().RequestURI()).
		Send()
}

func (rt *Router) handleRootRedirect(ctx *fasthttp.RequestCtx) {
	ctx.Redirect("/mirrors", fasthttp.StatusMovedPermanently)
}

func (rt *Router) handlePing(ctx *fasthttp.RequestCtx) {
	ctx.WriteString("pong")
}

func (rt *Router) handleIndex(ctx *fasthttp.RequestCtx) {
	pathBuf := bytebufferpool.Get()
	defer bytebufferpool.Put(pathBuf)

	var record purl.Record
	var ok bool
	pathBuf.B, record, ok = rt.deps.pathResolver.Append(ctx.QueryArgs().Peek("path"), pathBuf.B)
	if !ok {
		ctx.Error("path not resolved", fasthttp.StatusNotFound)
		return
	}

	it, age, err := rt.deps.indexProvider.AllOrErr(ctx, record.Host, pathBuf.B)
	if err != nil {
		switch {
		case errors.Is(err, index.ErrNotFound):
			ctx.Error("index not found", fasthttp.StatusNotFound)
		case errors.Is(err, index.ErrUpstreamFailure):
			ctx.Error("upstream failure", fasthttp.StatusBadGateway)
		default:
			ctx.Error("internal server error", fasthttp.StatusInternalServerError)
		}
		return
	}

	ctx.Response.Header.Set("Cache-Control", "public, max-age="+strconv.Itoa(int(rt.deps.indexProvider.CacheTTL().Seconds())))
	ctx.Response.Header.Set("Age", strconv.Itoa(int(age.Seconds())))

	setHeaderNDJSON(ctx)

	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		enc := sonic.ConfigDefault.NewEncoder(w)

		for e := range it {
			if err := enc.Encode(e); err != nil {
				rt.deps.logger.Error().Err(err).Msg("index entry encode failed")
				continue
			}

			if err := w.Flush(); err != nil {
				rt.deps.logger.Warn().Err(err).Msg("connection aborted")
				return
			}
		}
	})
}

func (rt *Router) handleRedirect(ctx *fasthttp.RequestCtx) {
	path := ctx.Path()

	pathBuf := bytebufferpool.Get()
	defer bytebufferpool.Put(pathBuf)

	var record purl.Record
	var ok bool
	pathBuf.B, record, ok = rt.deps.pathResolver.Append(path, pathBuf.B)
	if !ok {
		ctx.Error("path not resolved", fasthttp.StatusNotFound)
		return
	}

	// Sniff Host header and select FQDN variant
	host := ctx.Request.Header.Host()
	// Strip port (keep IPv6 brackets intact)
	if n := bytes.LastIndexByte(host, ':'); n != -1 {
		if bytes.LastIndexByte(host, ']') < n {
			host = host[:n]
		}
	}

	fqdn := record.FQDN // default fallback
	if len(host) > 0 {
		switch {
		case bytes.EqualFold(host, rt.cfg.siteURLv4):
			fqdn = record.FQDNv4
		case bytes.EqualFold(host, rt.cfg.siteURLv6):
			fqdn = record.FQDNv6
		}
	}

	uri := ctx.Request.URI()

	// Sniff proto
	if proto := ctx.Request.Header.Peek(rt.cfg.protoHeader); bytes.Equal(protoHTTPBytes, proto) {
		uri.SetSchemeBytes(protoHTTPBytes)
	} else if bytes.Equal(protoHTTPSBytes, proto) {
		uri.SetSchemeBytes(protoHTTPSBytes)
	}

	uri.SetHost(fqdn)

	ctx.RedirectBytes(uri.FullURI(), fasthttp.StatusMovedPermanently)
}

func (rt *Router) handleMirrorsRequest(ctx *fasthttp.RequestCtx) {
	it, age := rt.deps.mirrorGetter.All()

	ctx.Response.Header.Set("Cache-Control", "public, max-age="+strconv.Itoa(int(rt.deps.mirrorGetter.CacheTTL().Seconds())))
	ctx.Response.Header.Set("Age", strconv.Itoa(int(age.Seconds())))

	setHeaderNDJSON(ctx)

	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		enc := sonic.ConfigDefault.NewEncoder(w)

		for m := range it {
			if err := enc.Encode(m); err != nil {
				rt.deps.logger.Error().Err(err).Msg("mirror encode failed")
				continue
			}

			if err := w.Flush(); err != nil {
				rt.deps.logger.Warn().Err(err).Msg("connection aborted")
				return
			}
		}
	})
}

func (rt *Router) handleMirrorzRequest(ctx *fasthttp.RequestCtx) {
	mirrorz, age, err := rt.deps.mirrorzProvider.Mirrorz()
	if err != nil {
		rt.deps.logger.Error().Err(err).Msg("mirrorz generation failed")
		ctx.Error("internal server error", fasthttp.StatusInternalServerError)
		return
	}

	ctx.Response.Header.Set("Cache-Control", "public, max-age="+strconv.Itoa(int(rt.deps.mirrorzProvider.CacheTTL().Seconds())))
	ctx.Response.Header.Set("Age", strconv.Itoa(int(age.Seconds())))

	setHeaderJSON(ctx)

	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		enc := sonic.ConfigDefault.NewEncoder(w)
		if err := enc.Encode(mirrorz); err != nil {
			rt.deps.logger.Error().Err(err).Msg("mirrorz encode failed")
		}
	})
}

func (rt *Router) handleMirrorzHead(ctx *fasthttp.RequestCtx) {
	// TODO: handle as health check endpoint
	ctx.SetStatusCode(fasthttp.StatusOK)
}

func (rt *Router) handleBanMe(ctx *fasthttp.RequestCtx) {
	ctx.SetStatusCode(fasthttp.StatusNoContent)
}
