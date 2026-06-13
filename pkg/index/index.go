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

func EntryTypeFromString(s string) EntryType {
	switch s {
	case "file":
		return File
	case "directory":
		return Directory
	default:
		fallthrough
	case "other":
		return Other
	}
}

func (t EntryType) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.String())
}

func (t *EntryType) UnmarshalJSON(v []byte) error {
	sv := string(v)
	if len(sv) < 2 {
		return errors.New("payload too short")
	}
	*t = EntryTypeFromString(sv[1 : len(sv)-1])
	return nil
}

type Entry struct {
	Name  string    `json:"name"`
	Size  int64     `json:"size"`
	Mtime int64     `json:"mtime"`
	Type  EntryType `json:"type"`
}
