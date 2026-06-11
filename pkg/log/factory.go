package log

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/diode"
)

type LoggerConfig interface {
	Level() zerolog.Level
	Output() string
	PullInterval() time.Duration
	BufferSize() int
}

func NewLogger(cfg LoggerConfig) (zerolog.Logger, func(), error) {
	output := cfg.Output()
	if cfg.Level() == zerolog.Disabled {
		return zerolog.New(io.Discard), func() {}, nil
	} else {
		switch strings.ToLower(output) {
		case "":
			fallthrough
		case "stderr":
			return zerolog.New(os.Stderr), func() {}, nil
		case "stdout":
			return zerolog.New(os.Stdout), func() {}, nil
		default:
			f, err := os.OpenFile(
				output,
				os.O_APPEND|os.O_CREATE|os.O_WRONLY,
				0600,
			)
			if err != nil {
				return zerolog.Logger{}, nil, fmt.Errorf("NewLogger: failed to open log file: %w", err)
			}

			dw := diode.NewWriter(f, cfg.BufferSize(), cfg.PullInterval(), nil)

			return zerolog.New(dw), func() {
				dw.Close()
				f.Close()
			}, nil
		}
	}
}
