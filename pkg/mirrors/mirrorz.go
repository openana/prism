package mirrors

import (
	"bytes"
	"errors"
	"strconv"

	"github.com/docker/go-units"
)

// Mirrorz represents the MirrorZ JSON format v1.7.
// See https://github.com/mirrorz-org/mirrorz.
var MzVersion = MirrorzVersion{
	X: 1,
	Y: 7,
}

type MirrorzVersion struct {
	X int
	Y int
}

func (mv MirrorzVersion) MarshalJSON() ([]byte, error) {
	var buf []byte

	buf = append(buf, strconv.Itoa(mv.X)...)
	buf = append(buf, '.')
	buf = append(buf, strconv.Itoa(mv.Y)...)

	return buf, nil
}

func (mv *MirrorzVersion) UnmarshalJSON(v []byte) error {
	idx := bytes.IndexByte(v, '.')
	if idx < 0 {
		return errors.New("bad version")
	}

	x, err := strconv.Atoi(string(v[:idx]))
	if err != nil {
		return err
	}
	y, err := strconv.Atoi(string(v[idx+1:]))
	if err != nil {
		return err
	}

	mv.X = x
	mv.Y = y
	return nil
}

type Mirrorz struct {
	Version MirrorzVersion `json:"version"`
	Site    Site           `json:"site"`
	Info    []Info         `json:"info"`
	Mirrors []MirrorzEntry `json:"mirrors"`
}

// Site holds the global metadata about one mirror site.
// Only URL and Abbr are mandatory per the MirrorZ spec.
type Site struct {
	URL          string `json:"url"`
	Logo         string `json:"logo,omitempty"`
	LogoDarkmode string `json:"logo_darkmode,omitempty"`
	Abbr         string `json:"abbr"`
	Name         string `json:"name,omitempty"`
	Homepage     string `json:"homepage,omitempty"`
	Issue        string `json:"issue,omitempty"`
	Request      string `json:"request,omitempty"`
	Email        string `json:"email,omitempty"`
	Group        string `json:"group,omitempty"`
	Disk         string `json:"disk,omitempty"`
	Note         string `json:"note,omitempty"`
	Big          string `json:"big,omitempty"`
	Disable      bool   `json:"disable,omitempty"`
}

// Info describes a category-view entry in the MirrorZ info list.
type Info struct {
	Distro   string   `json:"distro"`
	Category string   `json:"category"`
	URLs     []ISOURL `json:"urls"`
}

// ISOURL is a named URL used in Info entries.
type ISOURL struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// MirrorzEntry describes a single mirror in the mirrors list of a MirrorZ response.
type MirrorzEntry struct {
	Cname    string `json:"cname"`
	Desc     string `json:"desc,omitempty"`
	URL      string `json:"url,omitempty"`
	Status   string `json:"status,omitempty"`
	Help     string `json:"help,omitempty"`
	Upstream string `json:"upstream,omitempty"`
	Size     string `json:"size,omitempty"`
	Disable  bool   `json:"disable,omitempty"`
}

// BuildMirrorzStatus converts the internal Sync data into the MirrorZ status
// string format. Returns "U" if sync is nil.
//
// Format: main-status[timestamp][aux-status[timestamp]...]
//
//	Main:  S(success), Y(syncing), F(failed), P(paused), D(pending), U(unknown)
//	Aux:   X(next schedule), O(old successful, when syncing or failed)
func BuildMirrorzStatus(sync *Sync) string {
	if sync == nil {
		return "U"
	}

	var buf []byte

	// Main status with timestamp
	switch sync.Status {
	case Success:
		buf = append(buf, 'S')
		if sync.LastEnded > 0 {
			buf = strconv.AppendInt(buf, sync.LastEnded, 10)
		}
	case Syncing:
		buf = append(buf, 'Y')
		if sync.LastStarted > 0 {
			buf = strconv.AppendInt(buf, sync.LastStarted, 10)
		}
	case Failed:
		buf = append(buf, 'F')
		if sync.LastEnded > 0 {
			buf = strconv.AppendInt(buf, sync.LastEnded, 10)
		}
	case Paused:
		buf = append(buf, 'P')
		if sync.LastEnded > 0 {
			buf = strconv.AppendInt(buf, sync.LastEnded, 10)
		}
	case PreSyncing:
		buf = append(buf, 'D')
		if sync.LastStarted > 0 {
			buf = strconv.AppendInt(buf, sync.LastStarted, 10)
		}
	default:
		buf = append(buf, 'U')
	}

	// Auxiliary: next schedule timestamp
	if sync.NextSchedule > 0 {
		buf = append(buf, 'X')
		buf = strconv.AppendInt(buf, sync.NextSchedule, 10)
	}

	// Auxiliary: old successful timestamp (only when currently syncing or failed)
	if (sync.Status == Syncing || sync.Status == Failed) && sync.LastUpdate > 0 {
		buf = append(buf, 'O')
		buf = strconv.AppendInt(buf, sync.LastUpdate, 10)
	}

	return string(buf)
}

// mirrorzSize formats a byte count into a human-readable size string for MirrorZ output.
// Uses units.HumanSize; returns "" for non-positive sizes.
func mirrorzSize(size int64) string {
	if size <= 0 {
		return ""
	}
	return units.HumanSize(float64(size))
}
