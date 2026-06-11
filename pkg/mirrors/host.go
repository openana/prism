package mirrors

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"
)

type Host interface {
	Name() string
	FetchMirrors(ctx context.Context) ([]Mirror, error)
}

type HostConfig interface {
	IsHostConfig()
}

func BuildHost(cfg HostConfig, logger zerolog.Logger) (Host, error) {
	switch v := cfg.(type) {
	case TunasyncHostConfig:
		return NewTunasyncHost(v, logger), nil
	default:
		return nil, fmt.Errorf("BuildHost: unknown host type %T", cfg)
	}
}
