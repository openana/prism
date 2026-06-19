package mirrorz

import "github.com/google/wire"

var MirrorzSet = wire.NewSet(
	NewManager,
	wire.Bind(new(Provider), new(*Manager)),
)
