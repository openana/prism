package config

type Router struct {
	protoHeader    string
	remoteIPHeader string
}

func (cfg *Router) ProtoHeader() string {
	return cfg.protoHeader
}

func (cfg *Router) RemoteIPHeader() string {
	return cfg.remoteIPHeader
}

func (cfg *HTTP) ToRouter() *Router {
	rt := &Router{}

	if cfg.ProtoHeader == "" {
		rt.protoHeader = "X-Forwarded-Proto"
	} else {
		rt.protoHeader = cfg.ProtoHeader
	}

	if cfg.RemoteIPHeader == "" {
		rt.remoteIPHeader = "X-Forwarded-For"
	} else {
		rt.remoteIPHeader = cfg.RemoteIPHeader
	}

	return rt
}
