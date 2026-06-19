package config

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
)

type Config struct {
	Log Log `yaml:"log"`

	AccessLog Log `yaml:"access_log"`

	HTTP HTTP `yaml:"http"`

	Index Index `yaml:"index"`

	SyncStatus SyncStatus `yaml:"sync_status"`

	Hosts []Host `yaml:"hosts"`

	StaticMirrors []StaticMirror `yaml:"static_mirrors"`

	Site Site `yaml:"site"`

	ISOInfo []ISOInfo `yaml:"iso_info"`

	News NewsConfig `yaml:"news"`

	Links []LinkItem `yaml:"links"`
}

type LinkItem struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type NewsConfig struct {
	Dir string `yaml:"dir"`
}

type Log struct {
	Level        string `yaml:"level"`
	Output       string `yaml:"output"`
	PullInterval string `yaml:"pull_interval"`
	BufferSize   *int   `yaml:"buffer_size"`
}

type HTTP struct {
	Listen         string `yaml:"listen"`
	ProtoHeader    string `yaml:"proto_header"`
	RemoteIPHeader string `yaml:"remote_ip_header"`
	Concurrency    *int   `yaml:"concurrency"`
	KeepAlive      *bool  `yaml:"keepalive"`
	TCPKeepAlive   *bool  `yaml:"tcp_keepalive"`
}

type Index struct {
	CacheTTL      string `yaml:"cache_ttl"`
	CacheMaxBytes string `yaml:"cache_max_bytes"`
}

type SyncStatus struct {
	CacheTTL     string `yaml:"cache_ttl"`
	FetchTimeout string `yaml:"fetch_timeout"`
}

type Host struct {
	Name       string         `yaml:"name"`
	FQDN       string         `yaml:"fqdn"`
	Index      HostIndex      `yaml:"index"`
	SyncStatus HostSyncStatus `yaml:"sync_status"`
	Mirrors    []Mirror       `yaml:"mirrors"`
}

type HostIndex struct {
	Driver string     `yaml:"driver"`
	Nginx  IndexNginx `yaml:"nginx"`
}

type IndexNginx struct {
	Timeout string `yaml:"timeout"`
	BaseURL string `yaml:"base_url"`
}

type HostSyncStatus struct {
	Driver   string             `yaml:"driver"`
	CacheTTL string             `yaml:"ttl"`
	Tunasync SyncStatusTunasync `yaml:"tunasync"`
}

type SyncStatusTunasync struct {
	Endpoint string `yaml:"endpoint"`
	Timeout  string `yaml:"timeout"`
}

type Mirror struct {
	Name          string     `yaml:"name"`
	Desc          string     `yaml:"desc"`
	Type          string     `yaml:"type"`
	URLPrefix     string     `yaml:"url_prefix"`
	RealURLPrefix string     `yaml:"real_url_prefix"`
	Help          MirrorHelp `yaml:"help"`
}

type MirrorHelp struct {
	// off, auto, manual
	Mode string `yaml:"mode"`
	URL  string `yaml:"url"`
}

type StaticMirror struct {
	FQDN          string     `yaml:"fqdn"`
	Name          string     `yaml:"name"`
	Desc          string     `yaml:"desc"`
	Type          string     `yaml:"type"`
	URLPrefix     string     `yaml:"url_prefix"`
	RealURLPrefix string     `yaml:"real_url_prefix"`
	Help          MirrorHelp `yaml:"help"`
}

type Site struct {
	URL          string `yaml:"url"`
	URLv4        string `yaml:"url_v4"`
	URLv6        string `yaml:"url_v6"`
	Logo         string `yaml:"logo"`
	LogoDarkmode string `yaml:"logo_darkmode"`
	Abbr         string `yaml:"abbr"`
	Name         string `yaml:"name"`
	Homepage     string `yaml:"homepage"`
	Issue        string `yaml:"issue"`
	Request      string `yaml:"request"`
	Email        string `yaml:"email"`
	Group        string `yaml:"group"`
	Disk         string `yaml:"disk"`
	Note         string `yaml:"note"`
	Big          string `yaml:"big"`
	Disable      bool   `yaml:"disable"`
}

type ISOInfo struct {
	Distro   string   `yaml:"distro"`
	Category string   `yaml:"category"`
	URLs     []ISOURL `yaml:"urls"`
}

type ISOURL struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Load: %w", err)
	}
	defer f.Close()

	cfg := new(Config)

	if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
		return nil, fmt.Errorf("Load: %w", err)
	}

	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("Load: %w", err)
	}

	return cfg, nil
}

func (cfg *Config) validate() error {
	// Host correctness
	hosts := map[string]struct{}{}

	for i, host := range cfg.Hosts {
		if host.Name == "" {
			return fmt.Errorf("empty hosts[%d].name", i)
		}
		if host.FQDN == "" {
			return fmt.Errorf("empty hosts[%d].fqdn", i)
		}

		_, ok := hosts[host.Name]
		if ok {
			return fmt.Errorf("hosts[%q]: duplicate host name", host.Name)
		}
		hosts[host.Name] = struct{}{}

		// mirrors
		for j, m := range host.Mirrors {
			if m.Name == "" {
				return fmt.Errorf("empty hosts[%q].mirrors[%d].name", host.Name, j)
			}
		}
	}

	// Static mirrors
	for i, m := range cfg.StaticMirrors {
		if m.FQDN == "" {
			return fmt.Errorf("empty static_mirrors[%d].fqdn", i)
		}

		if m.Name == "" {
			return fmt.Errorf("empty static_mirrors[%d].name", i)
		}
	}

	// Site & ISOInfo (mirrorz)
	if cfg.Site.URL != "" || cfg.Site.Abbr != "" {
		if cfg.Site.URL == "" {
			return fmt.Errorf("site.url is required when site is configured")
		}
		if cfg.Site.Abbr == "" {
			return fmt.Errorf("site.abbr is required when site is configured")
		}
		if cfg.Site.URL[len(cfg.Site.URL)-1] == '/' {
			return fmt.Errorf("site.url must not end with '/'")
		}
		for i, info := range cfg.ISOInfo {
			if info.Distro == "" {
				return fmt.Errorf("empty iso_info[%d].distro", i)
			}
			if info.Category == "" {
				return fmt.Errorf("empty iso_info[%d].category", i)
			}
			for j, u := range info.URLs {
				if u.Name == "" {
					return fmt.Errorf("empty iso_info[%d].urls[%d].name", i, j)
				}
				if u.URL == "" {
					return fmt.Errorf("empty iso_info[%d].urls[%d].url", i, j)
				}
			}
		}
	}

	return nil
}
