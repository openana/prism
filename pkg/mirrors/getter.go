package mirrors

import (
	"iter"
	"time"
)

type Getter interface {
	// All returns an iterator over all mirrors and the age of the cached
	// data (time since it was last fetched from upstream).
	All() (iter.Seq[Mirror], time.Duration)
	// CacheTTL returns the configured cache time-to-live.
	CacheTTL() time.Duration
}
