package meta

import (
	"fmt"
)

var (
	Name              = "prism"
	BuildDate  string = "1970-01-01 00:00:00+00:00"
	CommitHash string = "0000000000000000000000000000000000000000"
	Version    string = "N/A"
	Platform   string = "N/A"
	GoVersion  string = "N/A"

	VersionString string
	UserAgent     string
	ServerName    string
)

func init() {
	version := fmt.Sprintf(
		"%s %s %s (%s %s)",
		Name,
		Version,
		firstN(CommitHash, 7),
		GoVersion,
		Platform,
	)

	versionShort := fmt.Sprintf(
		"%s %s",
		Name,
		Version,
	)

	VersionString = version
	UserAgent = versionShort
	ServerName = versionShort
}

func firstN(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
