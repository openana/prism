package mirrorz

import (
	"time"

	"github.com/openana/prism/pkg/mirrors"
	"github.com/openana/prism/pkg/mirrors/cname"
	"github.com/rs/zerolog"
)

type Provider interface {
	Mirrorz() (*Mirrorz, time.Duration, error)
	CacheTTL() time.Duration
}

// Implemented by web.WebServer
type HelpURLProvider interface {
	HelpURL(name string) string
}

type Config interface {
	Site() Site
	Info() []Info
}

type Manager struct {
	cfg struct {
		site Site
		info []Info
	}

	deps struct {
		mirrorGetter mirrors.Getter
		helpProvider HelpURLProvider
		logger       zerolog.Logger
	}
}

func NewManager(cfg Config, mirrorGetter mirrors.Getter, helpProvider HelpURLProvider, logger zerolog.Logger) *Manager {
	mgr := &Manager{}

	mgr.cfg.site = cfg.Site()
	mgr.cfg.info = cfg.Info()

	mgr.deps.mirrorGetter = mirrorGetter
	mgr.deps.helpProvider = helpProvider
	mgr.deps.logger = logger.With().Str("module", "mirrorz.Manager").Logger()

	return mgr
}

func (mgr *Manager) Mirrorz() (*Mirrorz, time.Duration, error) {
	it, age := mgr.deps.mirrorGetter.All()

	entries := make([]MirrorzEntry, 0)
	for m := range it {
		entry := MirrorzEntry{
			Cname: cname.Cname(m.Name),
		}

		if m.Metadata != nil {
			switch m.Metadata.Type {
			case mirrors.Rsync:
			case mirrors.Git:
			case mirrors.Proxy:
			default:
				continue
			}
			entry.Desc = m.Metadata.Desc
			entry.URL = m.Metadata.URL
		}

		if m.Sync != nil {
			entry.Status = BuildMirrorzStatus(m.Sync)
			entry.Upstream = m.Sync.Upstream
			entry.Size = mirrorzSize(m.Sync.Size)
		} else {
			entry.Status = "U"
			entry.Disable = true
		}

		// Populate help URL from the web server's help registry
		if helpURL := mgr.deps.helpProvider.HelpURL(m.Name); helpURL != "" {
			entry.Help = helpURL
		}

		entries = append(entries, entry)
	}

	return &Mirrorz{
		Version: MzVersion,
		Site:    mgr.cfg.site,
		Info:    mgr.cfg.info,
		Mirrors: entries,
	}, age, nil
}

func (mgr *Manager) CacheTTL() time.Duration {
	return mgr.deps.mirrorGetter.CacheTTL()
}
