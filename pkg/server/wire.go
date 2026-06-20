//go:build wireinject
// +build wireinject

package server

import (
	"github.com/google/wire"
	"github.com/openana/prism/pkg/config"
	"github.com/openana/prism/pkg/index"
	"github.com/openana/prism/pkg/log"
	"github.com/openana/prism/pkg/mirrors"
	"github.com/openana/prism/pkg/mirrorz"
	"github.com/openana/prism/pkg/router"
	"github.com/openana/prism/pkg/web"

	purl "github.com/openana/prism/pkg/url"
)

func InitializeServer(cfg *config.Config) (*Server, func(), error) {
	wire.Build(
		NewServer,

		// Config interface providers
		ProvideServerConfig,
		ProvideRouterConfig,
		ProvideLoggerConfig,
		ProvideAccessLoggerConfig,
		ProvideTrieResolverConfig,
		ProvideMirrorManagerConfig,
		ProvideMirrorzManagerConfig,
		ProvideCachedIndexProviderConfig,
		ProvideWebServerConfig,

		// Modules
		purl.URLSet,
		router.RouterSet,
		mirrors.MirrorSet,
		mirrorz.MirrorzSet,
		log.LogSet,
		index.IndexSet,
		web.WebSet,

		// Bind web.Server to mirrorz.HelpURLProvider
		wire.Bind(new(mirrorz.HelpURLProvider), new(*web.Server)),
	)
	return nil, nil, nil
}

func ProvideServerConfig(cfg *config.Config) (ServerConfig, error) {
	return cfg.ToServer()
}

func ProvideRouterConfig(cfg *config.Config) router.RouterConfig {
	return cfg.ToRouter()
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

func ProvideMirrorzManagerConfig(cfg *config.Config) (mirrorz.Config, error) {
	return cfg.ToMirrorzManager()
}

func ProvideCachedIndexProviderConfig(cfg *config.Config) (index.CachedProviderConfig, error) {
	return cfg.ToCachedProvider()
}

func ProvideWebServerConfig(cfg *config.Config) web.ServerConfig {
	return cfg.ToWebServer()
}
