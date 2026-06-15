package mirrors

import (
	"iter"
	"time"
)

type Getter interface {
	// All returns an iterator over all mirrors and the age of the cached
	// data (time since it was last fetched from upstream).
	All() (iter.Seq[Mirror], time.Duration)
	// Mirrorz returns the MirrorZ JSON response, the age of the cached
	// data, and any error.
	Mirrorz() (*Mirrorz, time.Duration, error)
	// CacheTTL returns the configured cache time-to-live.
	CacheTTL() time.Duration
}
