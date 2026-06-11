package router

import (
	"github.com/valyala/fasthttp"
)

func setHeaderNDJSON(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.SetContentType("application/x-ndjson")
	ctx.Response.Header.Set("Transfer-Encoding", "chunked")
}

func setHeaderJSON(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.SetContentType("application/json")
}
