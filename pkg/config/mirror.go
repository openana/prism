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

	return &MirrorTunasyncHost{
		name:     name,
		endpoint: cfg.Endpoint,
	}, nil
}

func (cfg *MirrorTunasyncHost) IsHostConfig()    {}
func (cfg *MirrorTunasyncHost) Name() string     { return cfg.name }
func (cfg *MirrorTunasyncHost) Endpoint() string { return cfg.endpoint }

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
			// Help URL
			helpURL := ""
			switch m.Help.Mode {
			case "off":
			case "auto":
				helpURL = cfg.Misc.HelpURLPrefix + m.Name
			case "manual":
				helpURL = m.Help.URL
			default:
				return nil, fmt.Errorf("unknown hosts[%q].mirrors[%q].help.mode: %q", host.Name, m.Name, m.Help.Mode)
			}

			baseMirrors[m.Name] = mirrors.Mirror{
				Name: m.Name,
				Metadata: &mirrors.Metadata{
					Desc:    m.Desc,
					URL:     m.URLPrefix,
					HelpURL: helpURL,
					Type:    mirrors.TypeFromString(m.Type),
				},
			}
		}
	}

	// Static mirrors have higher precedence
	for _, m := range cfg.StaticMirrors {
		// Help URL
		helpURL := ""
		switch m.Help.Mode {
		case "off":
		case "auto":
			helpURL = cfg.Misc.HelpURLPrefix + m.Name
		case "manual":
			helpURL = m.Help.URL
		default:
			return nil, fmt.Errorf("unknown static_mirrors[%q].help.mode: %q", m.Name, m.Help.Mode)
		}

		baseMirrors[m.Name] = mirrors.Mirror{
			Name: m.Name,
			Metadata: &mirrors.Metadata{
				Desc:    m.Desc,
				URL:     m.URLPrefix,
				HelpURL: helpURL,
				Type:    mirrors.TypeFromString(m.Type),
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

	// Mirrorz site
	var mirrorzSite *mirrors.Site
	if cfg.Mirrorz.Site.Url != "" || cfg.Mirrorz.Site.Abbr != "" {
		mirrorzSite = &mirrors.Site{
			Url:          cfg.Mirrorz.Site.Url,
			Logo:         cfg.Mirrorz.Site.Logo,
			LogoDarkmode: cfg.Mirrorz.Site.LogoDarkmode,
			Abbr:         cfg.Mirrorz.Site.Abbr,
			Name:         cfg.Mirrorz.Site.Name,
			Homepage:     cfg.Mirrorz.Site.Homepage,
			Issue:        cfg.Mirrorz.Site.Issue,
			Request:      cfg.Mirrorz.Site.Request,
			Email:        cfg.Mirrorz.Site.Email,
			Group:        cfg.Mirrorz.Site.Group,
			Disk:         cfg.Mirrorz.Site.Disk,
			Note:         cfg.Mirrorz.Site.Note,
			Big:          cfg.Mirrorz.Site.Big,
			Disable:      cfg.Mirrorz.Site.Disable,
		}
	}

	// Mirrorz info
	mirrorzInfo := make([]mirrors.Info, 0, len(cfg.Mirrorz.Info))
	for _, info := range cfg.Mirrorz.Info {
		urls := make([]mirrors.MirrorzURL, 0, len(info.Urls))
		for _, u := range info.Urls {
			urls = append(urls, mirrors.MirrorzURL{
				Name: u.Name,
				Url:  u.Url,
			})
		}
		mirrorzInfo = append(mirrorzInfo, mirrors.Info{
			Distro:   info.Distro,
			Category: info.Category,
			Urls:     urls,
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
