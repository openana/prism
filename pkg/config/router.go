package config

type Router struct {
	protoHeader    string
	remoteIPHeader string
	siteURL        string
	siteURLv4      string
	siteURLv6      string
}

func (cfg *Router) ProtoHeader() string {
	return cfg.protoHeader
}

func (cfg *Router) RemoteIPHeader() string {
	return cfg.remoteIPHeader
}

func (cfg *Router) SiteURL() string {
	return cfg.siteURL
}

func (cfg *Router) SiteURLv4() string {
	return cfg.siteURLv4
}

func (cfg *Router) SiteURLv6() string {
	return cfg.siteURLv6
}

func (cfg *Config) ToRouter() *Router {
	rt := &Router{}

	if cfg.HTTP.ProtoHeader == "" {
		rt.protoHeader = "X-Forwarded-Proto"
	} else {
		rt.protoHeader = cfg.HTTP.ProtoHeader
	}

	if cfg.HTTP.RemoteIPHeader == "" {
		rt.remoteIPHeader = "X-Forwarded-For"
	} else {
		rt.remoteIPHeader = cfg.HTTP.RemoteIPHeader
	}

	rt.siteURL = cfg.Site.URL
	rt.siteURLv4 = cfg.Site.URLv4
	rt.siteURLv6 = cfg.Site.URLv6

	return rt
}
