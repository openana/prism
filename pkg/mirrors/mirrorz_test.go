package mirrors

import (
	"testing"
)

func TestBuildMirrorzStatus_NilSync(t *testing.T) {
	if got := BuildMirrorzStatus(nil); got != "U" {
		t.Errorf("BuildMirrorzStatus(nil) = %q, want %q", got, "U")
	}
}

func TestBuildMirrorzStatus_Success(t *testing.T) {
	sync := &Sync{
		Status:       Success,
		LastEnded:    1778201981,
		NextSchedule: 1780703762,
	}
	want := "S1778201981X1780703762"
	if got := BuildMirrorzStatus(sync); got != want {
		t.Errorf("BuildMirrorzStatus(success) = %q, want %q", got, want)
	}
}

func TestBuildMirrorzStatus_Syncing(t *testing.T) {
	sync := &Sync{
		Status:       Syncing,
		LastStarted:  1781267143,
		LastUpdate:   1781274491,
		NextSchedule: 1781296091,
	}
	want := "Y1781267143X1781296091O1781274491"
	if got := BuildMirrorzStatus(sync); got != want {
		t.Errorf("BuildMirrorzStatus(syncing) = %q, want %q", got, want)
	}
}

func TestBuildMirrorzStatus_Failed(t *testing.T) {
	sync := &Sync{
		Status:     Failed,
		LastEnded:  1780682162,
		LastUpdate: 1778201981,
	}
	want := "F1780682162O1778201981"
	if got := BuildMirrorzStatus(sync); got != want {
		t.Errorf("BuildMirrorzStatus(failed) = %q, want %q", got, want)
	}
}

func TestBuildMirrorzStatus_Paused(t *testing.T) {
	sync := &Sync{
		Status:    Paused,
		LastEnded: 1780682162,
	}
	want := "P1780682162"
	if got := BuildMirrorzStatus(sync); got != want {
		t.Errorf("BuildMirrorzStatus(paused) = %q, want %q", got, want)
	}
}

func TestBuildMirrorzStatus_PreSyncing(t *testing.T) {
	sync := &Sync{
		Status:      PreSyncing,
		LastStarted: 1780681628,
	}
	want := "D1780681628"
	if got := BuildMirrorzStatus(sync); got != want {
		t.Errorf("BuildMirrorzStatus(pre-syncing) = %q, want %q", got, want)
	}
}

func TestBuildMirrorzStatus_Unknown(t *testing.T) {
	sync := &Sync{
		Status: UnknownStatus,
	}
	if got := BuildMirrorzStatus(sync); got != "U" {
		t.Errorf("BuildMirrorzStatus(unknown) = %q, want %q", got, "U")
	}
}

func TestBuildMirrorzStatus_AllZeros(t *testing.T) {
	sync := &Sync{
		Status: Success,
	}
	if got := BuildMirrorzStatus(sync); got != "S" {
		t.Errorf("BuildMirrorzStatus(success with zeros) = %q, want %q", got, "S")
	}
}

func TestMirrorzSize(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{0, ""},
		{-1, ""},
	}
	for _, tt := range tests {
		if got := mirrorzSize(tt.size); got != tt.want {
			t.Errorf("mirrorzSize(%d) = %q, want %q", tt.size, got, tt.want)
		}
	}
	// Non-zero value should produce a non-empty string
	if got := mirrorzSize(596 * 1024 * 1024 * 1024); got == "" {
		t.Error("mirrorzSize(596G) should not be empty")
	}
	// Small positive values should also produce output
	if got := mirrorzSize(1024); got == "" {
		t.Error("mirrorzSize(1024) should not be empty")
	}
}
