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

	Misc Misc `yaml:"misc"`

	Mirrorz Mirrorz `yaml:"mirrorz"`
}

type Log struct {
	Level        string `yaml:"level"`
	Output       string `yaml:"output"`
	PullInterval string `yaml:"pull_interval"`
	BufferSize   *int   `yaml:"buffer_size"`
}

type HTTP struct {
	Listen      string `yaml:"listen"`
	ProtoHeader string `yaml:"proto_header"`
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

type Misc struct {
	HelpURLPrefix string `yaml:"help_url_prefix"`
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

type Mirrorz struct {
	Site MirrorzSite   `yaml:"site"`
	Info []MirrorzInfo `yaml:"info"`
}

type MirrorzSite struct {
	Url          string `yaml:"url"`
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

type MirrorzInfo struct {
	Distro   string       `yaml:"distro"`
	Category string       `yaml:"category"`
	Urls     []MirrorzURL `yaml:"urls"`
}

type MirrorzURL struct {
	Name string `yaml:"name"`
	Url  string `yaml:"url"`
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

	// Mirrorz
	if cfg.Mirrorz.Site.Url != "" || cfg.Mirrorz.Site.Abbr != "" {
		if cfg.Mirrorz.Site.Url == "" {
			return fmt.Errorf("mirrorz.site.url is required when mirrorz is configured")
		}
		if cfg.Mirrorz.Site.Abbr == "" {
			return fmt.Errorf("mirrorz.site.abbr is required when mirrorz is configured")
		}
		if cfg.Mirrorz.Site.Url[len(cfg.Mirrorz.Site.Url)-1] == '/' {
			return fmt.Errorf("mirrorz.site.url must not end with '/'")
		}
		for i, info := range cfg.Mirrorz.Info {
			if info.Distro == "" {
				return fmt.Errorf("empty mirrorz.info[%d].distro", i)
			}
			if info.Category == "" {
				return fmt.Errorf("empty mirrorz.info[%d].category", i)
			}
			for j, u := range info.Urls {
				if u.Name == "" {
					return fmt.Errorf("empty mirrorz.info[%d].urls[%d].name", i, j)
				}
				if u.Url == "" {
					return fmt.Errorf("empty mirrorz.info[%d].urls[%d].url", i, j)
				}
			}
		}
	}

	return nil
}
