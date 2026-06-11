package config

import (
	"errors"
)

type Server struct {
	listen string
}

func (cfg *Server) Listen() string {
	return cfg.listen
}

func (cfg *Config) ToServer() (*Server, error) {
	if cfg.HTTP.Listen == "" {
		return nil, errors.New("empty http.listen")
	}

	return &Server{
		listen: cfg.HTTP.Listen,
	}, nil
}
