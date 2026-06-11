package router

import "github.com/google/wire"

var RouterSet = wire.NewSet(
	NewRouter,
	wire.Bind(new(Handler), new(*Router)),
)
