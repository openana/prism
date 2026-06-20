package config

import (
	"errors"
	"time"

	"github.com/docker/go-units"
	"github.com/valyala/fasthttp"
)

type Server struct {
	listen             string
	concurrency        int
	keepAlive          bool
	tcpKeepAlive       bool
	readTimeout        time.Duration
	writeTimeout       time.Duration
	idleTimeout        time.Duration
	maxRequestBodySize int
	maxConnsPerIP      int
	reduceMemoryUsage  bool
}

func (cfg *Server) Listen() string              { return cfg.listen }
func (cfg *Server) Concurrency() int            { return cfg.concurrency }
func (cfg *Server) KeepAlive() bool             { return cfg.keepAlive }
func (cfg *Server) TCPKeepAlive() bool          { return cfg.tcpKeepAlive }
func (cfg *Server) ReadTimeout() time.Duration  { return cfg.readTimeout }
func (cfg *Server) WriteTimeout() time.Duration { return cfg.writeTimeout }
func (cfg *Server) IdleTimeout() time.Duration  { return cfg.idleTimeout }
func (cfg *Server) MaxRequestBodySize() int     { return cfg.maxRequestBodySize }
func (cfg *Server) MaxConnsPerIP() int          { return cfg.maxConnsPerIP }
func (cfg *Server) ReduceMemoryUsage() bool     { return cfg.reduceMemoryUsage }

const (
	defaultReadTimeout  = 30 * time.Second
	defaultWriteTimeout = 60 * time.Second
	defaultIdleTimeout  = 120 * time.Second
	defaultMaxBodySize  = 4 * 1024 * 1024 // 4 MB
)

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

	readTimeout := parseDuration(cfg.HTTP.ReadTimeout, defaultReadTimeout)
	writeTimeout := parseDuration(cfg.HTTP.WriteTimeout, defaultWriteTimeout)
	idleTimeout := parseDuration(cfg.HTTP.IdleTimeout, defaultIdleTimeout)

	maxRequestBodySize := defaultMaxBodySize
	if cfg.HTTP.MaxRequestBodySize != "" {
		v, err := units.RAMInBytes(cfg.HTTP.MaxRequestBodySize)
		if err != nil {
			return nil, errors.New("bad http.max_request_body_size: " + err.Error())
		}
		maxRequestBodySize = int(v)
	}

	return &Server{
		listen:             cfg.HTTP.Listen,
		concurrency:        concurrency,
		keepAlive:          keepAlive,
		tcpKeepAlive:       tcpKeepAlive,
		readTimeout:        readTimeout,
		writeTimeout:       writeTimeout,
		idleTimeout:        idleTimeout,
		maxRequestBodySize: maxRequestBodySize,
		maxConnsPerIP:      cfg.HTTP.MaxConnsPerIP,
		reduceMemoryUsage:  boolPtr(cfg.HTTP.ReduceMemoryUsage),
	}, nil
}

func parseDuration(s string, defaultVal time.Duration) time.Duration {
	if s == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return defaultVal
	}
	return d
}

func boolPtr(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
