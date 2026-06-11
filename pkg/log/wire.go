package log

import (
	"github.com/google/wire"
	"github.com/rs/zerolog"
)

type AccessLogger zerolog.Logger

type AccessLoggerConfig LoggerConfig

func ProvideAccessLogger(cfg AccessLoggerConfig) (AccessLogger, func(), error) {
	l, f, e := NewLogger(LoggerConfig(cfg))
	return AccessLogger(l), f, e
}

var LogSet = wire.NewSet(
	NewLogger,
	ProvideAccessLogger,
)
