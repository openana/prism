package server

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/openana/prism/pkg/meta"
	"github.com/openana/prism/pkg/router"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
)

type ServerConfig interface {
	Listen() string
	Concurrency() int
	KeepAlive() bool
	TCPKeepAlive() bool
	ReadTimeout() time.Duration
	WriteTimeout() time.Duration
	IdleTimeout() time.Duration
	MaxRequestBodySize() int
	MaxConnsPerIP() int
	ReduceMemoryUsage() bool
}

const (
	stateRunning int32 = iota
	stateStopping
	stateStopped
)

type Server struct {
	state atomic.Int32

	cfg struct {
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

	router    router.Handler
	http      *fasthttp.Server
	listenErr chan error

	logger zerolog.Logger
}

func NewServer(cfg ServerConfig, router router.Handler, logger zerolog.Logger) *Server {
	srv := &Server{}

	srv.cfg.listen = cfg.Listen()
	srv.cfg.concurrency = cfg.Concurrency()
	srv.cfg.keepAlive = cfg.KeepAlive()
	srv.cfg.tcpKeepAlive = cfg.TCPKeepAlive()
	srv.cfg.readTimeout = cfg.ReadTimeout()
	srv.cfg.writeTimeout = cfg.WriteTimeout()
	srv.cfg.idleTimeout = cfg.IdleTimeout()
	srv.cfg.maxRequestBodySize = cfg.MaxRequestBodySize()
	srv.cfg.maxConnsPerIP = cfg.MaxConnsPerIP()
	srv.cfg.reduceMemoryUsage = cfg.ReduceMemoryUsage()

	srv.router = router
	srv.listenErr = make(chan error, 1)
	srv.logger = logger.With().Str("module", "server.Server").Logger()
	srv.logger.Debug().Str("listen", srv.cfg.listen).Msg("server created")

	return srv
}

func (srv *Server) Run(ctx context.Context) error {
	srv.http = &fasthttp.Server{
		Name:               meta.ServerName,
		Handler:            srv.router.HandleRequest,
		Concurrency:        srv.cfg.concurrency,
		DisableKeepalive:   !srv.cfg.keepAlive,
		TCPKeepalive:       srv.cfg.tcpKeepAlive,
		ReadTimeout:        srv.cfg.readTimeout,
		WriteTimeout:       srv.cfg.writeTimeout,
		IdleTimeout:        srv.cfg.idleTimeout,
		MaxRequestBodySize: srv.cfg.maxRequestBodySize,
		MaxConnsPerIP:      srv.cfg.maxConnsPerIP,
		ReduceMemoryUsage:  srv.cfg.reduceMemoryUsage,
	}

	srv.state.Store(stateRunning)
	srv.logger.Info().
		Str("listen", srv.cfg.listen).
		Dur("read_timeout", srv.cfg.readTimeout).
		Dur("write_timeout", srv.cfg.writeTimeout).
		Dur("idle_timeout", srv.cfg.idleTimeout).
		Msg("http listen")

	go func() {
		if err := srv.http.ListenAndServe(srv.cfg.listen); err != nil {
			if srv.state.Load() == stateStopping {
				return // expected shutdown, not an error
			}
			srv.logger.Error().Err(err).Str("listen", srv.cfg.listen).Msg("listen failed")
			srv.listenErr <- err
		}
	}()

	return nil
}

func (srv *Server) Stop(ctx context.Context) error {
	srv.state.Store(stateStopping)
	srv.logger.Debug().Msg("stopping server")
	if err := srv.http.ShutdownWithContext(ctx); err != nil {
		srv.logger.Warn().Err(err).Msg("http server shutdown failed")
		srv.state.Store(stateStopped)
		return err
	}

	srv.state.Store(stateStopped)
	srv.logger.Info().Msg("server stopped")
	return nil
}
