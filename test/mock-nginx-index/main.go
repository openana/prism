// Mock Nginx index serves stable random json index in Nginx format.
// The result is consistent for the same path.
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"time"

	"github.com/valyala/fasthttp"
)

const (
	maxEntries = 64
)

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	flag.Parse()

	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("Mock Nginx Index server listening on %s\n", addr)
	if err := fasthttp.ListenAndServe(addr, handleIndex); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func handleIndex(ctx *fasthttp.RequestCtx) {
	seed := sha256.Sum256(ctx.Path())
	rnd := rand.New(rand.NewChaCha8(seed))

	// 1/5 chance of 403.
	if rnd.IntN(5) == 0 {
		ctx.Error(`{"code":403,"request":""}`, fasthttp.StatusForbidden)
		return
	}

	ctx.SetContentType("application/json")
	ctx.SetStatusCode(fasthttp.StatusOK)

	n := rnd.IntN(maxEntries)
	if n == 0 {
		ctx.WriteString("[]\n")
		return
	}

	ctx.WriteString("[\n")
	writeLine(ctx, rnd)
	for range n - 1 {
		ctx.WriteString(",\n")
		writeLine(ctx, rnd)
	}
	ctx.WriteString("\n]\n")
}

func writeLine(ctx *fasthttp.RequestCtx, rnd *rand.Rand) {
	name := fileName(rnd)
	mtime := formatTime(rnd)
	switch rnd.IntN(3) {
	case 0:
		// File
		ctx.WriteString(`{ "name":"`)
		ctx.WriteString(name)
		ctx.WriteString(`", "type":"file", "mtime":"`)
		ctx.WriteString(mtime)
		ctx.WriteString(`", "size":`)
		ctx.WriteString(strconv.FormatInt(rnd.Int64(), 10))
		ctx.WriteString(" }")
	case 1:
		// Directory
		ctx.WriteString(`{ "name":"`)
		ctx.WriteString(name)
		ctx.WriteString(`", "type":"directory", "mtime":"`)
		ctx.WriteString(mtime)
		ctx.WriteString(`" }`)
	default:
		// Other
		ctx.WriteString(`{ "name":"`)
		ctx.WriteString(name)
		ctx.WriteString(`", "type":"other", "mtime":"`)
		ctx.WriteString(mtime)
		ctx.WriteString(`" }`)
	}
}

var charset = []byte("abcdefghijklmnopqrstuvwxyz0123456789-")

func fileName(rnd *rand.Rand) string {
	nameLen := 3 + rnd.IntN(20)
	buf := make([]byte, nameLen)
	for i := range buf {
		buf[i] = charset[rnd.IntN(len(charset))]
	}
	return string(buf)
}

var gmt = time.FixedZone("GMT", 0)

func formatTime(rnd *rand.Rand) string {
	t := time.Unix(rnd.Int64(), 0).In(gmt)
	return t.Format(time.RFC1123)
}
