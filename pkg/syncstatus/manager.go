package syncstatus

import (
	"iter"
)

type Manager interface {
	All() iter.Seq[Mirror]
	Get(name string) (m Mirror, ok bool)
}
