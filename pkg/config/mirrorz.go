package config

import (
	"github.com/openana/prism/pkg/mirrorz"
)

// MirrorzManager implements mirrorz.Config, providing site metadata and
// ISO info for the MirrorZ JSON response.
type MirrorzManager struct {
	site mirrorz.Site
	info []mirrorz.Info
}

func (cfg *MirrorzManager) Site() mirrorz.Site   { return cfg.site }
func (cfg *MirrorzManager) Info() []mirrorz.Info { return cfg.info }

// ToMirrorzManager builds the MirrorZ configuration from the top-level Config.
func (cfg *Config) ToMirrorzManager() (*MirrorzManager, error) {
	var site mirrorz.Site
	if cfg.Site.URL != "" || cfg.Site.Abbr != "" {
		site = mirrorz.Site{
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

	info := make([]mirrorz.Info, 0, len(cfg.ISOInfo))
	for _, iso := range cfg.ISOInfo {
		urls := make([]mirrorz.ISOURL, 0, len(iso.URLs))
		for _, u := range iso.URLs {
			urls = append(urls, mirrorz.ISOURL{
				Name: u.Name,
				URL:  u.URL,
			})
		}
		info = append(info, mirrorz.Info{
			Distro:   iso.Distro,
			Category: iso.Category,
			URLs:     urls,
		})
	}

	return &MirrorzManager{
		site: site,
		info: info,
	}, nil
}
