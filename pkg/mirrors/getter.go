package mirrors

import (
	"iter"
)

type Getter interface {
	All() iter.Seq[Mirror]
}
