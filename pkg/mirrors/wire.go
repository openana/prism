package mirrors

import "github.com/google/wire"

var MirrorSet = wire.NewSet(
	NewManager,
	wire.Bind(new(Getter), new(*Manager)),
)
