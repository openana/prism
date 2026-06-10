package index

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
)

// Provider and Fetcher should return these errors if applies.
var (
	ErrNotFound        = errors.New("not found")
	ErrUpstreamFailure = errors.New("upstream failure")
)

type Provider interface {
	AllOrErr(ctx context.Context, host string, path []byte) (iter.Seq[Entry], error)
}

type Fetcher interface {
	AllOrErr(ctx context.Context, path []byte) (iter.Seq[Entry], error)
}

type EntryType int8

const (
	Other EntryType = iota
	File
	Directory
)

func (t EntryType) String() string {
	switch t {
	case File:
		return "file"
	case Directory:
		return "directory"
	default:
		return "other"
	}
}

type Entry struct {
	Name  string
	Size  int64
	Mtime int64
	Type  EntryType
}

func (e *Entry) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		Mtime int64  `json:"mtime"`
		Size  int64  `json:"size"`
	}{
		Name:  e.Name,
		Type:  e.Type.String(),
		Mtime: e.Mtime,
		Size:  e.Size,
	})
}
