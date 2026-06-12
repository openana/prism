package log

import (
	"github.com/google/wire"
	"github.com/rs/zerolog"
)

type AccessLogger zerolog.Logger

type AccessLoggerConfig LoggerConfig

var LogSet = wire.NewSet(
	NewLogger,
	NewAccessLogger,
)
