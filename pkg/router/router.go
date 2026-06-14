package router

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"

	"github.com/openana/prism/pkg/index"
	"github.com/openana/prism/pkg/log"
	"github.com/openana/prism/pkg/mirrors"
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
}

// Router is the top level fasthttp handler provider.
type Router struct {
	cfg struct {
		protoHeader string
	}

	deps struct {
		logger        zerolog.Logger
		accessLogger  zerolog.Logger
		pathResolver  purl.Resolver
		mirrorGetter  mirrors.Getter
		indexProvider index.Provider
		webHandler    web.Handler
	}
}

func NewRouter(cfg RouterConfig, logger zerolog.Logger, accessLogger log.AccessLogger, pathResolver purl.Resolver, mirrorGetter mirrors.Getter, indexProvider index.Provider, webHandler web.Handler) *Router {
	eng := &Router{}

	eng.cfg.protoHeader = cfg.ProtoHeader()

	eng.deps.logger = logger.With().Str("module", "router.Router").Logger()
	eng.deps.accessLogger = zerolog.Logger(accessLogger)
	eng.deps.pathResolver = pathResolver
	eng.deps.mirrorGetter = mirrorGetter
	eng.deps.indexProvider = indexProvider
	eng.deps.webHandler = webHandler

	return eng
}

var (
	// "/"
	pathAPI    = []byte("api/")
	pathStatic = []byte("static/")
	pathPages  = []byte("pages/")

	// "pages/"
	pathMirrorsPage   = []byte("mirrors")
	pathDownloadsPage = []byte("downloads")

	// "api/"
	pathPing    = []byte("ping")
	pathIndex   = []byte("index")
	pathMirrors = []byte("mirrors")
	pathMirrorz = []byte("mirrorz")

	// headers
	protoHTTPBytes  = []byte("http")
	protoHTTPSBytes = []byte("https")
)

const (
	stateStart int = iota
	stateEndFail
	stateEndSuccess
	stateHandleAPI
	stateHandleIndex
	stateHandleRedirect
	stateHandleStatic
	stateHandlePages
)

func (eng *Router) HandleRequest(ctx *fasthttp.RequestCtx) {
	state := stateStart
	path := ctx.Path()
	errMsg := "unknown error"
	errStatus := fasthttp.StatusInternalServerError
	handleFail := func(ctx *fasthttp.RequestCtx) {
		ctx.Error(errMsg, errStatus)
	}

	for {
		switch state {
		case stateEndFail:
			handleFail(ctx)
			fallthrough

		case stateEndSuccess:
			// Log request
			eng.deps.accessLogger.Info().
				Int("status", ctx.Response.StatusCode()).
				Bytes("uri", ctx.URI().RequestURI()).
				Send()

			return

		case stateStart:
			if len(path) == 0 || path[0] != '/' {
				errMsg = "not found"
				errStatus = fasthttp.StatusNotFound
				state = stateEndFail
				continue
			}

			path = path[1:]

			if bytes.HasPrefix(path, pathAPI) {
				path = path[4:]
				state = stateHandleAPI
				continue
			}

			if bytes.HasPrefix(path, pathPages) {
				path = path[6:]
				state = stateHandlePages
				continue
			}

			if bytes.HasPrefix(path, pathStatic) {
				path = path[7:]
				state = stateHandleStatic
				continue
			}

			state = stateHandleRedirect
			continue

		case stateHandleAPI:
			if bytes.Equal(path, pathIndex) {
				state = stateHandleIndex
				continue
			}

			if bytes.Equal(path, pathMirrors) {
				eng.handleMirrorsRequest(ctx)
				state = stateEndSuccess
				continue
			}

			if bytes.Equal(path, pathMirrorz) {
				eng.handleMirrorzRequest(ctx)
				state = stateEndSuccess
				continue
			}

			if bytes.Equal(path, pathPing) {
				ctx.WriteString("pong")
				state = stateEndSuccess
				continue
			}

			errMsg = "not found"
			errStatus = fasthttp.StatusNotFound
			state = stateEndFail
			continue

		case stateHandleIndex:
			pathBuf := bytebufferpool.Get()
			defer bytebufferpool.Put(pathBuf)

			var record purl.Record
			var ok bool
			pathBuf.B, record, ok = eng.deps.pathResolver.Append(ctx.QueryArgs().Peek("path"), pathBuf.B)
			if !ok {
				errMsg = "path not resolved"
				errStatus = fasthttp.StatusNotFound
				state = stateEndFail
				continue
			}

			it, err := eng.deps.indexProvider.AllOrErr(ctx, record.Host, pathBuf.B)
			if err != nil {
				switch {
				case errors.Is(err, index.ErrNotFound):
					errMsg = "index not found"
					errStatus = fasthttp.StatusNotFound
				case errors.Is(err, index.ErrUpstreamFailure):
					errMsg = "upstream failure"
					errStatus = fasthttp.StatusBadGateway
				default:
					errMsg = "internal server error"
					errStatus = fasthttp.StatusInternalServerError
				}
				state = stateEndFail
				continue
			}

			setHeaderNDJSON(ctx)

			ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
				enc := json.NewEncoder(w)

				for e := range it {
					if err := enc.Encode(e); err != nil {
						eng.deps.logger.Error().Err(err).Msg("index entry encode failed")
						continue
					}

					if err := w.Flush(); err != nil {
						eng.deps.logger.Warn().Err(err).Msg("connection aborted")
						return
					}
				}
			})

			state = stateEndSuccess
			continue

		case stateHandleRedirect:
			// Reset path
			path = ctx.Path()

			pathBuf := bytebufferpool.Get()
			defer bytebufferpool.Put(pathBuf)

			var record purl.Record
			var ok bool
			pathBuf.B, record, ok = eng.deps.pathResolver.Append(path, pathBuf.B)
			if !ok {
				errMsg = "path not resolved"
				errStatus = fasthttp.StatusNotFound
				state = stateEndFail
				continue
			}

			uri := ctx.Request.URI()

			// Sniff proto
			if proto := ctx.Request.Header.Peek(eng.cfg.protoHeader); bytes.Equal(protoHTTPBytes, proto) {
				uri.SetSchemeBytes(protoHTTPBytes)
			} else if bytes.Equal(protoHTTPSBytes, proto) {
				uri.SetSchemeBytes(protoHTTPSBytes)
			}

			uri.SetHost(record.FQDN)

			ctx.RedirectBytes(uri.FullURI(), fasthttp.StatusMovedPermanently)

			state = stateEndSuccess
			continue

		case stateHandleStatic:
			// TODO: static assets
			ctx.Response.SetStatusCode(fasthttp.StatusNotImplemented)
			state = stateEndSuccess
			continue

		case stateHandlePages:
			if bytes.Equal(path, pathMirrorsPage) {
				eng.deps.webHandler.HandleMirrors(ctx)
				state = stateEndSuccess
				continue
			}

			if bytes.Equal(path, pathDownloadsPage) {
				eng.deps.webHandler.HandleDownloads(ctx)
				state = stateEndSuccess
				continue
			}

			errMsg = "page not found"
			errStatus = fasthttp.StatusNotFound
			state = stateEndFail
			continue
		}
	}
}

func (eng *Router) handleMirrorsRequest(ctx *fasthttp.RequestCtx) {
	setHeaderNDJSON(ctx)

	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		enc := json.NewEncoder(w)

		for m := range eng.deps.mirrorGetter.All() {
			if err := enc.Encode(m); err != nil {
				eng.deps.logger.Error().Err(err).Msg("mirror encode failed")
				continue
			}

			if err := w.Flush(); err != nil {
				eng.deps.logger.Warn().Err(err).Msg("connection aborted")
				return
			}
		}
	})
}

func (eng *Router) handleMirrorzRequest(ctx *fasthttp.RequestCtx) {
	// HEAD requests: return 200 with no body, skip mirrorz generation
	if ctx.IsHead() {
		ctx.SetStatusCode(fasthttp.StatusOK)
		return
	}

	mirrorz, err := eng.deps.mirrorGetter.Mirrorz()
	if err != nil {
		eng.deps.logger.Error().Err(err).Msg("mirrorz generation failed")
		ctx.Error("internal server error", fasthttp.StatusInternalServerError)
		return
	}

	setHeaderJSON(ctx)

	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		enc := json.NewEncoder(w)
		if err := enc.Encode(mirrorz); err != nil {
			eng.deps.logger.Error().Err(err).Msg("mirrorz encode failed")
		}
	})
}
