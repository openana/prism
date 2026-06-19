package mirrors

import (
	"errors"

	"github.com/bytedance/sonic"
)

type SyncStatus int8

const (
	UnknownStatus SyncStatus = iota
	None
	Failed
	Success
	Syncing
	PreSyncing
	Paused
	Disabled
)

func (s SyncStatus) String() string {
	switch s {
	case None:
		return "none"
	case Failed:
		return "failed"
	case Success:
		return "success"
	case Syncing:
		return "syncing"
	case PreSyncing:
		return "pre-syncing"
	case Paused:
		return "paused"
	case Disabled:
		return "disabled"
	default:
		fallthrough
	case UnknownStatus:
		return "unknown"
	}
}

func SyncStatusFromString(s string) (ss SyncStatus) {
	switch s {
	case "none":
		ss = None
	case "failed":
		ss = Failed
	case "success":
		ss = Success
	case "syncing":
		ss = Syncing
	case "pre-syncing":
		ss = PreSyncing
	case "paused":
		ss = Paused
	case "disabled":
		ss = Disabled
	default:
		fallthrough
	case "unknown":
		ss = UnknownStatus
	}
	return
}

func (s SyncStatus) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(s.String())
}

func (s *SyncStatus) UnmarshalJSON(v []byte) error {
	sv := string(v)
	if len(sv) < 2 {
		return errors.New("payload too short")
	}
	*s = SyncStatusFromString(sv[1 : len(sv)-1])
	return nil
}

type Type int8

const (
	UnknownType Type = iota
	Rsync
	Git
	Proxy
	Redirect
)

func (s Type) String() string {
	switch s {
	case Rsync:
		return "rsync"
	case Git:
		return "git"
	case Proxy:
		return "proxy"
	case Redirect:
		return "redirect"
	default:
		fallthrough
	case UnknownType:
		return "unknown"
	}
}

func TypeFromString(s string) (ss Type) {
	switch s {
	case "rsync":
		ss = Rsync
	case "git":
		ss = Git
	case "proxy":
		ss = Proxy
	case "redirect":
		ss = Redirect
	default:
		fallthrough
	case "unknown":
		ss = UnknownType
	}
	return
}

func (s Type) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(s.String())
}

func (s *Type) UnmarshalJSON(v []byte) error {
	sv := string(v)
	if len(sv) < 2 {
		return errors.New("payload too short")
	}
	*s = TypeFromString(sv[1 : len(sv)-1])
	return nil
}

type Mirror struct {
	Name     string    `json:"name"`
	Metadata *Metadata `json:"metadata,omitempty"`
	Sync     *Sync     `json:"sync,omitempty"`
}

type Metadata struct {
	// "Arch Linux"
	Desc string `json:"desc"`
	// "/archlinux"
	URL  string `json:"url"`
	Type Type   `json:"type"`
}

type Sync struct {
	// "rsync://upstream.example.com/archlinux"
	Upstream string `json:"upstream"`
	// Unix timestamps
	LastUpdate   int64 `json:"last_update"`
	LastStarted  int64 `json:"last_started"`
	LastEnded    int64 `json:"last_ended"`
	NextSchedule int64 `json:"next_schedule"`

	// Bytes
	Size int64 `json:"size"`

	// ""
	Status SyncStatus `json:"status"`
}
