package statemgr

import (
	"context"
	"time"
)

type StateManager interface {
	Run(ctx context.Context) error
	Stop(ctx context.Context) error
	GetAllMirrors() (ms []Mirror, lastUpdate time.Time)
	GetMirror(name string) (m Mirror, lastUpdate time.Time, ok bool)
}
