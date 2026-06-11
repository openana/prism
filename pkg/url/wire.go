package url

import "github.com/google/wire"

var URLSet = wire.NewSet(
	NewTrieResolver,
	wire.Bind(new(Resolver), new(*TrieResolver)),
)
