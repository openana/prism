package web

import "github.com/google/wire"

var WebSet = wire.NewSet(
	NewServer,
	wire.Bind(new(Handler), new(*Server)),
)
