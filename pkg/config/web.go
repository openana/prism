package config

import (
	"github.com/openana/prism/pkg/web"
)

type WebServer struct {
	site        web.Site
	isoInfo     []web.ISOInfo
	helpMirrors []web.HelpMirrorConfig
	newsDir     string
}

func (cfg *WebServer) Site() web.Site {
	return cfg.site
}

func (cfg *WebServer) ISOInfo() []web.ISOInfo {
	return cfg.isoInfo
}

func (cfg *WebServer) HelpMirrors() []web.HelpMirrorConfig {
	return cfg.helpMirrors
}

func (cfg *WebServer) NewsDir() string {
	return cfg.newsDir
}

func (cfg *Config) ToWebServer() *WebServer {
	site := web.Site{
		Name:     cfg.Site.Name,
		URL:      cfg.Site.URL,
		URLv4:    cfg.Site.URLv4,
		URLv6:    cfg.Site.URLv6,
		Homepage: cfg.Site.Homepage,
		Issues:   cfg.Site.Issue,
		Request:  cfg.Site.Request,
		Email:    cfg.Site.Email,
		Group:    cfg.Site.Group,
		Disk:     cfg.Site.Disk,
		Note:     cfg.Site.Note,
		Big:      cfg.Site.Big,
	}

	links := make([]web.LinkItem, len(cfg.Links))
	for i, l := range cfg.Links {
		links[i] = web.LinkItem{
			Name: l.Name,
			URL:  l.URL,
		}
	}
	site.Links = links

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

	// Collect mirror helps
	seen := make(map[string]struct{})
	var helpMirrors []web.HelpMirrorConfig
	for _, host := range cfg.Hosts {
		for _, m := range host.Mirrors {
			if m.Help.Mode == "" {
				m.Help.Mode = "auto"
			}
			if m.Help.Mode == "auto" || m.Help.Mode == "manual" {
				if _, ok := seen[m.Name]; ok {
					continue
				}
				seen[m.Name] = struct{}{}
				helpMirrors = append(helpMirrors, web.HelpMirrorConfig{
					Name:      m.Name,
					Mode:      m.Help.Mode,
					URLPrefix: m.URLPrefix,
					HelpURL:   m.Help.URL,
				})
			}
		}
	}
	for _, m := range cfg.StaticMirrors {
		if m.Help.Mode == "" {
			m.Help.Mode = "auto"
		}
		if m.Help.Mode == "auto" || m.Help.Mode == "manual" {
			if _, ok := seen[m.Name]; ok {
				continue
			}
			seen[m.Name] = struct{}{}
			helpMirrors = append(helpMirrors, web.HelpMirrorConfig{
				Name:      m.Name,
				Mode:      m.Help.Mode,
				URLPrefix: m.URLPrefix,
				HelpURL:   m.Help.URL,
			})
		}
	}

	return &WebServer{
		site:        site,
		isoInfo:     isoInfo,
		helpMirrors: helpMirrors,
		newsDir:     cfg.News.Dir,
	}
}
