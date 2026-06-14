package config

import "github.com/openana/prism/pkg/web"

type WebServer struct {
	site    web.Site
	isoInfo []web.ISOInfo
}

func (cfg *WebServer) Site() web.Site {
	return cfg.site
}

func (cfg *WebServer) ISOInfo() []web.ISOInfo {
	return cfg.isoInfo
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

	isoInfo := make([]web.ISOInfo, len(cfg.ISOInfo))
	for i, info := range cfg.ISOInfo {
		urls := make([]web.ISODownload, len(info.URLs))
		for j, u := range info.URLs {
			urls[j] = web.ISODownload{
				Name: u.Name,
				URL:  u.URL,
			}
		}
		isoInfo[i] = web.ISOInfo{
			Distro:   info.Distro,
			Category: info.Category,
			URLs:     urls,
		}
	}

	return &WebServer{
		site:    site,
		isoInfo: isoInfo,
	}
}
