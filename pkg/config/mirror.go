package config

import (
	"fmt"
	"net/url"
	"time"

	"github.com/openana/prism/pkg/mirrors"
)

type MirrorTunasyncHost struct {
	name     string
	endpoint string
	timeout  time.Duration
}

func (cfg *SyncStatusTunasync) ToMirrorTunasyncHost(name string) (*MirrorTunasyncHost, error) {
	u, err := url.ParseRequestURI(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("bad endpoint: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("bad endpoint: unsupported scheme %q", u.Scheme)
	}

	if u.Host == "" {
		return nil, fmt.Errorf("bad endpoint: empty host")
	}

	var timeout time.Duration
	if cfg.Timeout == "" {
		timeout = 5 * time.Second
	} else {
		timeout, err = time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("bad timeout: %q", cfg.Timeout)
		}
	}

	return &MirrorTunasyncHost{
		name:     name,
		endpoint: cfg.Endpoint,
		timeout:  timeout,
	}, nil
}

func (cfg *MirrorTunasyncHost) IsHostConfig()          {}
func (cfg *MirrorTunasyncHost) Name() string           { return cfg.name }
func (cfg *MirrorTunasyncHost) Endpoint() string       { return cfg.endpoint }
func (cfg *MirrorTunasyncHost) Timeout() time.Duration { return cfg.timeout }

type MirrorManager struct {
	hosts        []mirrors.HostConfig
	cacheTTL     time.Duration
	fetchTimeout time.Duration
	baseMirrors  map[string]mirrors.Mirror
	mirrorzSite  *mirrors.Site
	mirrorzInfo  []mirrors.Info
}

func (cfg *MirrorManager) Hosts() []mirrors.HostConfig            { return cfg.hosts }
func (cfg *MirrorManager) CacheTTL() time.Duration                { return cfg.cacheTTL }
func (cfg *MirrorManager) FetchTimeout() time.Duration            { return cfg.fetchTimeout }
func (cfg *MirrorManager) BaseMirrors() map[string]mirrors.Mirror { return cfg.baseMirrors }
func (cfg *MirrorManager) MirrorzSite() *mirrors.Site             { return cfg.mirrorzSite }
func (cfg *MirrorManager) MirrorzInfo() []mirrors.Info            { return cfg.mirrorzInfo }

func (cfg *Config) ToMirrorManager() (*MirrorManager, error) {
	// Cache TTL
	if cfg.SyncStatus.CacheTTL == "" {
		return nil, fmt.Errorf("sync_status.cache_ttl not set")
	}

	ttl, err := time.ParseDuration(cfg.SyncStatus.CacheTTL)
	if err != nil {
		return nil, fmt.Errorf("bad sync_status.cache_ttl: %w", err)
	}

	// Fetch timeout
	var fetchTimeout time.Duration
	if cfg.SyncStatus.FetchTimeout == "" {
		fetchTimeout = 5 * time.Second
	} else {
		fetchTimeout, err = time.ParseDuration(cfg.SyncStatus.FetchTimeout)
		if err != nil {
			return nil, fmt.Errorf("bad sync_status.fetch_timeout: %w", err)
		}
	}

	// Base mirrors
	baseMirrors := make(map[string]mirrors.Mirror)
	for _, host := range cfg.Hosts {
		for _, m := range host.Mirrors {
			baseMirrors[m.Name] = mirrors.Mirror{
				Name: m.Name,
				Metadata: &mirrors.Metadata{
					Desc: m.Desc,
					URL:  m.URLPrefix,
					Type: mirrors.TypeFromString(m.Type),
				},
			}
		}
	}

	// Static mirrors have higher precedence
	for _, m := range cfg.StaticMirrors {
		baseMirrors[m.Name] = mirrors.Mirror{
			Name: m.Name,
			Metadata: &mirrors.Metadata{
				Desc: m.Desc,
				URL:  m.URLPrefix,
				Type: mirrors.TypeFromString(m.Type),
			},
		}
	}

	// hosts
	var hosts []mirrors.HostConfig

	for _, host := range cfg.Hosts {
		switch host.SyncStatus.Driver {
		case "tunasync":
			h, err := host.SyncStatus.Tunasync.ToMirrorTunasyncHost(host.Name)
			if err != nil {
				return nil, fmt.Errorf("hosts[%q].sync_status.tunasync: %w", host.Name, err)
			}
			hosts = append(hosts, h)
		default:
			return nil, fmt.Errorf("unsupported hosts[%q].sync_status.driver: %q", host.Name, host.SyncStatus.Driver)
		}
	}

	// Site
	var mirrorzSite *mirrors.Site
	if cfg.Site.URL != "" || cfg.Site.Abbr != "" {
		mirrorzSite = &mirrors.Site{
			URL:          cfg.Site.URL,
			Logo:         cfg.Site.Logo,
			LogoDarkmode: cfg.Site.LogoDarkmode,
			Abbr:         cfg.Site.Abbr,
			Name:         cfg.Site.Name,
			Homepage:     cfg.Site.Homepage,
			Issue:        cfg.Site.Issue,
			Request:      cfg.Site.Request,
			Email:        cfg.Site.Email,
			Group:        cfg.Site.Group,
			Disk:         cfg.Site.Disk,
			Note:         cfg.Site.Note,
			Big:          cfg.Site.Big,
			Disable:      cfg.Site.Disable,
		}
	}

	// ISOInfo
	mirrorzInfo := make([]mirrors.Info, 0, len(cfg.ISOInfo))
	for _, info := range cfg.ISOInfo {
		urls := make([]mirrors.ISOURL, 0, len(info.URLs))
		for _, u := range info.URLs {
			urls = append(urls, mirrors.ISOURL{
				Name: u.Name,
				URL:  u.URL,
			})
		}
		mirrorzInfo = append(mirrorzInfo, mirrors.Info{
			Distro:   info.Distro,
			Category: info.Category,
			URLs:     urls,
		})
	}

	return &MirrorManager{
		hosts:        hosts,
		cacheTTL:     ttl,
		fetchTimeout: fetchTimeout,
		baseMirrors:  baseMirrors,
		mirrorzSite:  mirrorzSite,
		mirrorzInfo:  mirrorzInfo,
	}, nil
}
