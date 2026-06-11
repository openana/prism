package index

import "github.com/google/wire"

var IndexSet = wire.NewSet(
	NewCachedProvider,
	wire.Bind(new(Provider), new(*CachedProvider)),
)
