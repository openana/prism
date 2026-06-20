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
		fqdnV4 := host.FQDNv4
		if fqdnV4 == "" {
			fqdnV4 = host.FQDN
		}
		fqdnV6 := host.FQDNv6
		if fqdnV6 == "" {
			fqdnV6 = host.FQDN
		}
		for _, m := range host.Mirrors {
			records[m.URLPrefix] = url.Record{
				Host:   host.Name,
				FQDN:   host.FQDN,
				FQDNv4: fqdnV4,
				FQDNv6: fqdnV6,
				Prefix: m.RealURLPrefix,
			}
		}
	}

	// Static mirrors have higher precedence
	for _, m := range cfg.StaticMirrors {
		fqdnV4 := m.FQDNv4
		if fqdnV4 == "" {
			fqdnV4 = m.FQDN
		}
		fqdnV6 := m.FQDNv6
		if fqdnV6 == "" {
			fqdnV6 = m.FQDN
		}
		records[m.URLPrefix] = url.Record{
			Host:   "",
			FQDN:   m.FQDN,
			FQDNv4: fqdnV4,
			FQDNv6: fqdnV6,
			Prefix: m.RealURLPrefix,
		}
	}

	return &TrieResolver{
		records: records,
	}
}
