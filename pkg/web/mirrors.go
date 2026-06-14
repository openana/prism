package web

import (
	"time"

	"github.com/openana/prism/pkg/mirrors"
)

type Mirror struct {
	Name       string
	URL        string
	Desc       string
	Type       string
	Help       string
	LastUpdate string
}

func FormatMirrors(src *mirrors.Mirror) Mirror {
	tgt := Mirror{
		Name: src.Name,
	}

	if src.Metadata != nil {
		tgt.URL = src.Metadata.URL
		tgt.Help = src.Metadata.HelpURL
		tgt.Type = src.Metadata.Type.String()
		tgt.Desc = src.Metadata.Desc
	}

	if src.Sync != nil {
		tgt.LastUpdate = time.Unix(src.Sync.LastUpdate, 0).UTC().Format(time.RFC3339)
	}

	return tgt
}

type MirrorPage struct {
	Mirrors []Mirror
}
