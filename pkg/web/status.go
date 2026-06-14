package web

import (
	"bufio"
	"time"

	"github.com/docker/go-units"
	"github.com/openana/prism/pkg/mirrors"
	"github.com/openana/prism/pkg/web/i18n"
	"github.com/valyala/fasthttp"
)

type Status struct {
	Name         string
	Type         string
	LastUpdate   string
	LastStarted  string
	LastEnded    string
	NextSchedule string
	Upstream     string
	Status       string
	StatusClass  string
	Size         string
}

func FormatStatus(src *mirrors.Mirror) (Status, bool) {
	tgt := Status{
		Name: src.Name,
	}

	if src.Metadata != nil {
		tgt.Type = src.Metadata.Type.String()
	} else {
		return Status{}, false
	}

	if src.Sync != nil {
		tgt.LastUpdate = time.Unix(src.Sync.LastUpdate, 0).Format(time.RFC3339)
		tgt.LastStarted = time.Unix(src.Sync.LastStarted, 0).Format(time.RFC3339)
		tgt.LastEnded = time.Unix(src.Sync.LastEnded, 0).Format(time.RFC3339)
		tgt.NextSchedule = time.Unix(src.Sync.NextSchedule, 0).Format(time.RFC3339)
		tgt.Upstream = src.Sync.Upstream
		tgt.Status = src.Sync.Status.String()
		tgt.StatusClass = statusClass(src.Sync.Status)
		tgt.Size = units.HumanSize(float64(src.Sync.Size))
	} else {
		return Status{}, false
	}

	return tgt, true
}

type StatusPage struct {
	Locale  *i18n.Locale
	Mirrors []Status
}

func statusClass(s mirrors.SyncStatus) string {
	switch s {
	case mirrors.Failed:
		return "status-failed"
	case mirrors.Syncing, mirrors.PreSyncing:
		return "status-syncing"
	case mirrors.Paused:
		return "status-paused"
	default:
		return ""
	}
}

func (s *Server) HandleStatus(ctx *fasthttp.RequestCtx) {
	var mirrors []Status
	for m := range s.deps.mirrorGetter.All() {
		st, ok := FormatStatus(&m)
		if !ok {
			continue
		}
		mirrors = append(mirrors, st)
	}

	ctx.SetContentType("text/html; charset=utf-8")
	ctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		if err := s.pages.status.ExecuteTemplate(w, "base", StatusPage{Locale: s.resolveLocale(ctx), Mirrors: mirrors}); err != nil {
			s.deps.logger.Error().Err(err).Msg("failed to render template")
		}
		w.Flush()
	})
}
