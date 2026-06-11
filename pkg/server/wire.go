//go:build wireinject
// +build wireinject

package server

import (
	"github.com/google/wire"
	"github.com/openana/prism/pkg/config"
	"github.com/openana/prism/pkg/index"
	"github.com/openana/prism/pkg/log"
	"github.com/openana/prism/pkg/mirrors"
	"github.com/openana/prism/pkg/router"

	purl "github.com/openana/prism/pkg/url"
)

func InitializeServer(cfg *config.Config) (srv *Server, cleanup func(), err error) {
	wire.Build(
		NewServer,

		// Config interface providers
		ProvideServerConfig,
		ProvideRouterConfig,
		ProvideLoggerConfig,
		ProvideAccessLoggerConfig,
		ProvideTrieResolverConfig,
		ProvideMirrorManagerConfig,
		ProvideCachedIndexProviderConfig,

		// Modules
		purl.URLSet,
		router.RouterSet,
		mirrors.MirrorSet,
		log.LogSet,
		index.IndexSet,
	)
	return
}

func ProvideServerConfig(cfg *config.Config) (ServerConfig, error) {
	return cfg.ToServer()
}

func ProvideRouterConfig(cfg *config.Config) (router.RouterConfig, error) {
	return cfg.HTTP.ToRouter()
}

func ProvideLoggerConfig(cfg *config.Config) (log.LoggerConfig, error) {
	return cfg.Log.ToLogger()
}

func ProvideAccessLoggerConfig(cfg *config.Config) (log.AccessLoggerConfig, error) {
	return cfg.AccessLog.ToLogger()
}

func ProvideTrieResolverConfig(cfg *config.Config) purl.TrieResolverConfig {
	return cfg.ToTrieResolver()
}

func ProvideMirrorManagerConfig(cfg *config.Config) (mirrors.ManagerConfig, error) {
	return cfg.ToMirrorManager()
}

func ProvideCachedIndexProviderConfig(cfg *config.Config) (index.CachedProviderConfig, error) {
	return cfg.ToCachedProvider()
}
