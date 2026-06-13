package config

import "github.com/openana/prism/pkg/url"

type TrieResolver struct {
	records map[string]url.Record
}

func (cfg *TrieResolver) Records() map[string]url.Record {
	return cfg.records
}

func (cfg *Config) ToTrieResolver() *TrieResolver {
	records := make(map[string]url.Record)
	for _, host := range cfg.Hosts {
		for _, m := range host.Mirrors {
			records[m.URLPrefix] = url.Record{
				Host:   host.Name,
				FQDN:   host.FQDN,
				Prefix: m.RealURLPrefix,
			}
		}
	}

	// Static mirrors have higher precedence
	for _, m := range cfg.StaticMirrors {
		records[m.URLPrefix] = url.Record{
			Host:   "",
			FQDN:   m.FQDN,
			Prefix: m.RealURLPrefix,
		}
	}

	return &TrieResolver{
		records: records,
	}
}
