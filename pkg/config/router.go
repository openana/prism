package config

import (
	"errors"
)

type Router struct {
	protoHeader string
}

func (cfg *Router) ProtoHeader() string {
	return cfg.protoHeader
}

func (cfg *HTTP) ToRouter() (*Router, error) {
	if cfg.ProtoHeader == "" {
		return nil, errors.New("empty http.proto_header")
	}

	return &Router{protoHeader: cfg.ProtoHeader}, nil
}
