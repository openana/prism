package config

import (
	"errors"

	"github.com/valyala/fasthttp"
)

type Server struct {
	listen       string
	concurrency  int
	keepAlive    bool
	tcpKeepAlive bool
}

func (cfg *Server) Listen() string {
	return cfg.listen
}

func (cfg *Server) Concurrency() int {
	return cfg.concurrency
}

func (cfg *Server) KeepAlive() bool {
	return cfg.keepAlive
}

func (cfg *Server) TCPKeepAlive() bool {
	return cfg.tcpKeepAlive
}

func (cfg *Config) ToServer() (*Server, error) {
	if cfg.HTTP.Listen == "" {
		return nil, errors.New("empty http.listen")
	}

	keepAlive := true // default: keep-alive enabled (matches fasthttp default)
	if cfg.HTTP.KeepAlive != nil {
		keepAlive = *cfg.HTTP.KeepAlive
	}

	tcpKeepAlive := false // default: TCP keep-alive disabled
	if cfg.HTTP.TCPKeepAlive != nil {
		tcpKeepAlive = *cfg.HTTP.TCPKeepAlive
	}

	concurrency := fasthttp.DefaultConcurrency
	if cfg.HTTP.Concurrency != nil && *cfg.HTTP.Concurrency > 0 {
		concurrency = *cfg.HTTP.Concurrency
	}

	return &Server{
		listen:       cfg.HTTP.Listen,
		concurrency:  concurrency,
		keepAlive:    keepAlive,
		tcpKeepAlive: tcpKeepAlive,
	}, nil
}
