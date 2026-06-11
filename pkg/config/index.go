package config

import (
	"fmt"
	"net/url"
	"time"

	"github.com/docker/go-units"
	"github.com/openana/prism/pkg/index"
)

type NginxFetcher struct {
	baseURL string
	timeout time.Duration
}

func (cfg *IndexNginx) ToFetcher() (*NginxFetcher, error) {
	// Timeout
	var timeout time.Duration
	var err error
	if cfg.Timeout == "" {
		timeout = 5 * time.Second
	} else {
		timeout, err = time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("bad timeout: %q", cfg.Timeout)
		}
	}

	// Base URL
	u, err := url.ParseRequestURI(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("bad base_url: %w", err)
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("bad base_url: unsupported scheme %q", u.Scheme)
	}

	if u.Host == "" {
		return nil, fmt.Errorf("bad base_url: empty host")
	}

	if cfg.BaseURL[len(cfg.BaseURL)-1] != '/' {
		cfg.BaseURL = cfg.BaseURL + "/"
	}

	return &NginxFetcher{
		baseURL: cfg.BaseURL,
		timeout: timeout,
	}, nil
}

func (cfg *NginxFetcher) IsFetcherConfig()       {}
func (cfg *NginxFetcher) Timeout() time.Duration { return cfg.timeout }
func (cfg *NginxFetcher) BaseURL() string        { return cfg.baseURL }

type CacheProvider struct {
	fetchers map[string]index.FetcherConfig
	ttl      time.Duration
	maxBytes int
}

func (cfg *CacheProvider) Fetchers() map[string]index.FetcherConfig { return cfg.fetchers }
func (cfg *CacheProvider) TTL() time.Duration                       { return cfg.ttl }
func (cfg *CacheProvider) MaxBytes() int                            { return cfg.maxBytes }

func (cfg *Config) ToCachedProvider() (*CacheProvider, error) {
	// Cache config
	if cfg.Index.CacheTTL == "" {
		return nil, fmt.Errorf("index.cache_ttl not set")
	}

	ttl, err := time.ParseDuration(cfg.Index.CacheTTL)
	if err != nil {
		return nil, fmt.Errorf("bad index.cache_ttl: %w", err)
	}

	if cfg.Index.CacheMaxBytes == "" {
		return nil, fmt.Errorf("index.cache_max_bytes not set")
	}

	maxBytes, err := units.FromHumanSize(cfg.Index.CacheMaxBytes)
	if err != nil {
		return nil, fmt.Errorf("bad index.cache_max_bytes: %w", err)
	}

	// Fetcher configs
	fetchers := make(map[string]index.FetcherConfig)
	for _, host := range cfg.Hosts {
		switch host.Index.Driver {
		case "nginx":
			f, err := host.Index.Nginx.ToFetcher()
			if err != nil {
				return nil, fmt.Errorf("hosts[%q].index.nginx: %w", host.Name, err)
			}
			fetchers[host.Name] = f
		default:
			return nil, fmt.Errorf("unsupported hosts[%q].index.driver: %q", host.Name, host.Index.Driver)
		}
	}

	return &CacheProvider{
		fetchers: fetchers,
		ttl:      ttl,
		maxBytes: int(maxBytes),
	}, nil
}
