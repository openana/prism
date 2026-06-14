package config

import "github.com/openana/prism/pkg/web"

type WebServer struct {
	site web.Site
}

func (cfg *WebServer) Site() web.Site {
	return cfg.site
}

func (cfg *Config) ToWebServer() *WebServer {
	site := web.Site{
		Name:     cfg.Site.Name,
		URL:      cfg.Site.URL,
		Homepage: cfg.Site.Homepage,
		Issues:   cfg.Site.Issue,
		Request:  cfg.Site.Request,
		Email:    cfg.Site.Email,
		Group:    cfg.Site.Group,
		Disk:     cfg.Site.Disk,
		Note:     cfg.Site.Note,
		Big:      cfg.Site.Big,
	}

	return &WebServer{
		site: site,
	}
}
