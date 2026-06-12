package mirrors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/docker/go-units"
	"github.com/openana/prism/pkg/meta"
	"github.com/rs/zerolog"
)

type TunasyncMirror struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	LastUpdate   int64  `json:"last_update_ts"`
	LastStarted  int64  `json:"last_started_ts"`
	LastEnded    int64  `json:"last_ended_ts"`
	NextSchedule int64  `json:"next_schedule_ts"`
	Upstream     string `json:"upstream"`
	Size         string `json:"size"`
}

type TunasyncHostConfig interface {
	HostConfig
	Name() string
	Endpoint() string
}

type TunasyncHost struct {
	name     string
	endpoint string
	logger   zerolog.Logger
}

func NewTunasyncHost(cfg TunasyncHostConfig, logger zerolog.Logger) *TunasyncHost {
	return &TunasyncHost{
		name:     cfg.Name(),
		endpoint: cfg.Endpoint(),
		logger:   logger.With().Str("module", "mirrors.TunasyncHost:"+cfg.Name()).Logger(),
	}
}

func (h *TunasyncHost) Name() string {
	return h.name
}

func (h *TunasyncHost) FetchMirrors(ctx context.Context) ([]Mirror, error) {
	h.logger.Debug().Str("endpoint", h.endpoint).Msg("fetching mirrors")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.endpoint, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", meta.UserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TunasyncHost: unexpected status: %d", resp.StatusCode)
	}

	var tms []TunasyncMirror

	if err := json.NewDecoder(resp.Body).Decode(&tms); err != nil {
		return nil, err
	}

	mirrors := make([]Mirror, 0, len(tms))

	for _, tm := range tms {
		size, err := units.FromHumanSize(tm.Size)
		if err != nil {
			h.logger.Warn().Err(err).Msg("parse size error")
			size = -1
		}

		mirrors = append(mirrors, Mirror{
			Name: tm.Name,
			SyncStatus: &SyncStatus{
				Status:       tm.Status,
				LastUpdate:   tm.LastUpdate,
				LastStarted:  tm.LastStarted,
				LastEnded:    tm.LastEnded,
				NextSchedule: tm.NextSchedule,
				Upstream:     tm.Upstream,
				Size:         size,
			},
		})
	}

	return mirrors, nil
}
