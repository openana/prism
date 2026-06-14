package web

import (
	"io/fs"
	"mime"
	"path/filepath"

	"github.com/valyala/fasthttp"
)

const (
	contentTypeOctetStream = "application/octet-stream"
)

// Static files are named <original>.<first-8-of-sha256>.<ext>
func (s *Server) HandleStatic(ctx *fasthttp.RequestCtx) {
	ctx.Response.Header.Set("Cache-Control", "public, max-age=31536000, immutable")

	path, ok := ctx.UserValue("path").(string)
	if !ok {
		ctx.Error("not found", fasthttp.StatusNotFound)
	}

	fileBytes, err := fs.ReadFile(staticFS, path)
	if err != nil {
		ctx.Error("not found", fasthttp.StatusNotFound)
	}

	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = contentTypeOctetStream
	}

	ctx.SetContentType(contentType)

	ctx.SetBody(fileBytes)
}
