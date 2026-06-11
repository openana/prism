package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

type Logger struct {
	level        zerolog.Level
	output       string
	pullInterval time.Duration
	bufferSize   int
}

func (cfg *Log) ToLogger() (*Logger, error) {
	// level
	var level zerolog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = zerolog.DebugLevel
	case "":
		fallthrough // default to "info"
	case "info":
		level = zerolog.InfoLevel
	case "warn":
		level = zerolog.WarnLevel
	case "error":
		level = zerolog.ErrorLevel
	case "fatal":
		level = zerolog.FatalLevel
	case "panic":
		level = zerolog.PanicLevel
	case "trace":
		level = zerolog.TraceLevel
	case "disabled":
		level = zerolog.Disabled
	default:
		return nil, fmt.Errorf("unknown level: %q", cfg.Level)
	}

	var pullInterval time.Duration
	var err error
	if cfg.PullInterval == "" {
		pullInterval = 1 * time.Second
	} else {
		pullInterval, err = time.ParseDuration(cfg.PullInterval)
		if err != nil {
			return nil, fmt.Errorf("bad pull_interval: %q", cfg.PullInterval)
		}
	}

	var bufferSize int
	if cfg.BufferSize == nil {
		bufferSize = 1000
	} else {
		bufferSize = *cfg.BufferSize
	}

	return &Logger{
		level:        level,
		output:       cfg.Output,
		pullInterval: pullInterval,
		bufferSize:   bufferSize,
	}, nil
}

func (cfg *Logger) Level() zerolog.Level        { return cfg.level }
func (cfg *Logger) Output() string              { return cfg.output }
func (cfg *Logger) PullInterval() time.Duration { return cfg.pullInterval }
func (cfg *Logger) BufferSize() int             { return cfg.bufferSize }
