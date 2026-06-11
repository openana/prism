package index

import (
	"context"
	"fmt"
	"iter"

	"github.com/rs/zerolog"
)

type Fetcher interface {
	AllOrErr(ctx context.Context, path []byte) (iter.Seq[Entry], error)
}

type FetcherConfig interface {
	IsFetcherConfig()
}

// For mock tests
type GenericFetcherConfig struct {
	F Fetcher
}

func (GenericFetcherConfig) IsFetcherConfig() {}

func BuildFetcher(cfg FetcherConfig, logger zerolog.Logger) (Fetcher, error) {
	switch v := cfg.(type) {
	case NginxFetcherConfig:
		return NewNginxFetcher(v, logger), nil
	case GenericFetcherConfig:
		return v.F, nil
	default:
		return nil, fmt.Errorf("BuildFetcher: unknown fetcher type %T", cfg)
	}
}
