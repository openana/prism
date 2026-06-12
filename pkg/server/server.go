package server

import (
	"context"
	"sync/atomic"

	"github.com/openana/prism/pkg/meta"
	"github.com/openana/prism/pkg/router"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
)

type ServerConfig interface {
	Listen() string
}

type Server struct {
	state atomic.Int32

	cfg struct {
		listen string
	}

	router router.Handler
	http   *fasthttp.Server

	logger zerolog.Logger
}

func NewServer(cfg ServerConfig, router router.Handler, logger zerolog.Logger) *Server {
	srv := &Server{}

	srv.cfg.listen = cfg.Listen()

	srv.router = router
	srv.logger = logger.With().Str("module", "server.Server").Logger()

	return srv
}

func (srv *Server) Run(ctx context.Context) error {
	srv.http = &fasthttp.Server{
		// TODO: add more options via configuration if needed
		Name:    meta.ServerName,
		Handler: srv.router.HandleRequest,
	}

	srv.logger.Info().Str("listen", srv.cfg.listen).Msg("http listen")

	go func() {
		if err := srv.http.ListenAndServe(srv.cfg.listen); err != nil {
			srv.logger.Error().Err(err).Str("listen", srv.cfg.listen).Msg("listen failed")
		}
	}()

	return nil
}

func (srv *Server) Stop(ctx context.Context) error {
	if err := srv.http.ShutdownWithContext(ctx); err != nil {
		srv.logger.Warn().Err(err).Msg("http server shutdown failed")
		return err
	}

	return nil
}
