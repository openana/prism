package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"

	"github.com/valyala/fasthttp"
)

//go:embed tunasync.json
var respBody []byte

func main() {
	port := flag.Int("port", 8080, "port to listen on")
	flag.Parse()

	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("Mock Tunasync server listening on %s\n", addr)
	if err := fasthttp.ListenAndServe(addr, func(ctx *fasthttp.RequestCtx) {
		ctx.Write(respBody)
	}); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
