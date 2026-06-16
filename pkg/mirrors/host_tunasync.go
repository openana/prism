package mirrors

import (
	"context"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/docker/go-units"
	"github.com/openana/prism/pkg/meta"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
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
	Timeout() time.Duration
}

type TunasyncHost struct {
	name     string
	endpoint string
	logger   zerolog.Logger
	client   *fasthttp.Client
}

func NewTunasyncHost(cfg TunasyncHostConfig, logger zerolog.Logger) *TunasyncHost {
	return &TunasyncHost{
		name:     cfg.Name(),
		endpoint: cfg.Endpoint(),
		logger:   logger.With().Str("module", "mirrors.TunasyncHost:"+cfg.Name()).Logger(),
		client: &fasthttp.Client{
			ReadTimeout:         cfg.Timeout(),
			MaxIdleConnDuration: 90 * time.Second,
		},
	}
}

func (h *TunasyncHost) Name() string {
	return h.name
}

func (h *TunasyncHost) FetchMirrors(ctx context.Context) ([]Mirror, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	h.logger.Debug().Str("endpoint", h.endpoint).Msg("fetching mirrors")

	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	req.SetRequestURI(h.endpoint)
	req.Header.SetMethod(fasthttp.MethodGet)
	req.Header.Set("User-Agent", meta.UserAgent)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(h.client.ReadTimeout)
	}

	if err := h.client.DoDeadline(req, resp, deadline); err != nil {
		h.logger.Warn().Err(err).Str("endpoint", h.endpoint).Msg("http request failed")
		return nil, err
	}

	if resp.StatusCode() != fasthttp.StatusOK {
		h.logger.Warn().Int("status", resp.StatusCode()).Str("endpoint", h.endpoint).Msg("unexpected upstream status")
		return nil, fmt.Errorf("TunasyncHost: unexpected status: %d", resp.StatusCode())
	}

	var tms []TunasyncMirror

	if err := sonic.Unmarshal(resp.Body(), &tms); err != nil {
		h.logger.Error().Err(err).Str("endpoint", h.endpoint).Msg("json decode failed")
		return nil, err
	}

	mirrors := make([]Mirror, 0, len(tms))

	for _, tm := range tms {
		size, err := units.FromHumanSize(tm.Size)
		if err != nil {
			if tm.Size != "unknown" {
				h.logger.Warn().Err(err).Str("size", tm.Size).Msg("parse size error")
			}
			size = -1
		}

		mirrors = append(mirrors, Mirror{
			Name: tm.Name,
			Sync: &Sync{
				Status:       SyncStatusFromString(tm.Status),
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
