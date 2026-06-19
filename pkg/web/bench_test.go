// LLM usage: generated with deepseek-v4-pro and modified manually.
package web

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"iter"
	"slices"
	"testing"
	"time"

	"github.com/openana/prism/pkg/index"
	"github.com/openana/prism/pkg/mirrors"
	purl "github.com/openana/prism/pkg/url"
	"github.com/rs/zerolog"
	"github.com/valyala/fasthttp"
)

// ---------------------------------------------------------------------------
// mock implementations
// ---------------------------------------------------------------------------

type mockMirrorGetter struct {
	mirrors  []mirrors.Mirror
	cacheTTL time.Duration
	age      time.Duration
}

func (m *mockMirrorGetter) All() (iter.Seq[mirrors.Mirror], time.Duration) {
	return slices.Values(m.mirrors), m.age
}

func (m *mockMirrorGetter) Mirrorz() (*mirrors.Mirrorz, time.Duration, error) {
	return nil, 0, nil
}

func (m *mockMirrorGetter) CacheTTL() time.Duration {
	return m.cacheTTL
}

type mockIndexProvider struct {
	entries  []index.Entry
	cacheTTL time.Duration
	age      time.Duration
}

func (m *mockIndexProvider) AllOrErr(_ context.Context, _ string, _ []byte) (iter.Seq[index.Entry], time.Duration, error) {
	return slices.Values(m.entries), m.age, nil
}

func (m *mockIndexProvider) CacheTTL() time.Duration {
	return m.cacheTTL
}

type mockPathResolver struct {
	prefix []byte
	record purl.Record
}

func (m *mockPathResolver) Append(path []byte, dst []byte) ([]byte, purl.Record, bool) {
	result := append(dst, m.prefix...)
	result = append(result, path...)
	return result, m.record, true
}

type mockServerConfig struct {
	site    Site
	isoInfo []ISOInfo
}

func (m *mockServerConfig) Site() Site                      { return m.site }
func (m *mockServerConfig) ISOInfo() []ISOInfo              { return m.isoInfo }
func (m *mockServerConfig) HelpMirrors() []HelpMirrorConfig { return nil }

// ---------------------------------------------------------------------------
// test data helpers
// ---------------------------------------------------------------------------

var mirrorNames = []string{
	"alpine", "archlinux", "centos", "debian", "fedora", "gentoo",
	"kali", "manjaro", "opensuse", "ubuntu",
}

func makeMirrors(n int) []mirrors.Mirror {
	ms := make([]mirrors.Mirror, n)
	for i := range n {
		nameIdx := i % len(mirrorNames)
		name := mirrorNames[nameIdx]
		if i >= len(mirrorNames) {
			name = fmt.Sprintf("%s-%d", name, i/len(mirrorNames))
		}
		ms[i] = mirrors.Mirror{
			Name: name,
			Metadata: &mirrors.Metadata{
				Desc: name + " mirror",
				URL:  "/" + name,
				Type: mirrors.Type(i%3 + 1), // cycle through Rsync(1), Git(2), Proxy(3)
			},
			Sync: &mirrors.Sync{
				Upstream:     "rsync://upstream.example.com/" + name,
				LastUpdate:   1700000000 + int64(i*3600),
				LastStarted:  1699999000 + int64(i*3600),
				LastEnded:    1700000000 + int64(i*3600),
				NextSchedule: 1700086400 + int64(i*3600),
				Size:         int64((i + 1) * 1024 * 1024 * 1024), // sizes in GiB
				Status:       mirrors.SyncStatus(i%7 + 1),         // cycle through all non-zero statuses
			},
		}
	}
	return ms
}

var entryNames = []string{
	"Packages", "Packages.gz", "Packages.xz", "Release", "InRelease",
	"Sources.gz", "Sources.xz", "binary-amd64", "binary-i386", "source",
	"Contents-amd64.gz", "Contents-i386.gz", "Translation-en", "Translation-zh",
	"by-hash", "current", "stable", "testing", "unstable",
}

func makeEntries(n int) []index.Entry {
	es := make([]index.Entry, n)
	for i := range n {
		nameIdx := i % len(entryNames)
		name := entryNames[nameIdx]
		if i >= len(entryNames) {
			name = fmt.Sprintf("%s-%d", name, i/len(entryNames))
		}
		// First 20% are directories, rest files
		eType := index.File
		if i < n/5 {
			eType = index.Directory
		}
		es[i] = index.Entry{
			Name:  name,
			Size:  int64((i + 1) * 1024),
			Mtime: 1700000000 + int64(i*60),
			Type:  eType,
		}
	}
	return es
}

func makeSite() Site {
	return Site{
		Name:     "Mirrors Benchmark",
		URL:      "https://mirrors.example.com",
		Homepage: "https://www.example.com",
		Issues:   "https://github.com/example/mirrors/issues",
		Request:  "https://github.com/example/mirrors/issues/new",
		Email:    "admin@example.com",
		Group:    "Example University",
		Disk:     "10TB",
		Note:     "Benchmark mirror site",
	}
}

func makeISOInfo() []ISOInfo {
	return []ISOInfo{
		{Distro: "ubuntu", Category: "os", URLs: []ISODownload{
			{Name: "Ubuntu 24.04 LTS Desktop", URL: "https://releases.ubuntu.com/24.04/ubuntu-24.04-desktop-amd64.iso"},
			{Name: "Ubuntu 24.04 LTS Server", URL: "https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso"},
		}},
		{Distro: "debian", Category: "os", URLs: []ISODownload{
			{Name: "Debian 12.0 DVD", URL: "https://cdimage.debian.org/debian-cd/12.0.0/amd64/iso-dvd/debian-12.0.0-amd64-DVD-1.iso"},
		}},
		{Distro: "fedora", Category: "os", URLs: []ISODownload{
			{Name: "Fedora 40 Workstation", URL: "https://download.fedoraproject.org/pub/fedora/linux/releases/40/Workstation/x86_64/iso/Fedora-Workstation-Live-x86_64-40.iso"},
		}},
		{Distro: "centos", Category: "os", URLs: []ISODownload{
			{Name: "CentOS Stream 9 DVD", URL: "https://mirrors.centos.org/mirrorlist?path=/9-stream/BaseOS/x86_64/iso/CentOS-Stream-9-latest-x86_64-dvd1.iso"},
		}},
		{Distro: "archlinux", Category: "os", URLs: []ISODownload{
			{Name: "Arch Linux ISO", URL: "https://archlinux.org/iso/latest/archlinux-x86_64.iso"},
		}},
		{Distro: "vscode", Category: "app", URLs: []ISODownload{
			{Name: "VS Code Linux x64", URL: "https://code.visualstudio.com/sha/download?build=stable&os=linux-x64"},
		}},
		{Distro: "jetbrains", Category: "app", URLs: []ISODownload{
			{Name: "IntelliJ IDEA Ultimate", URL: "https://www.jetbrains.com/idea/download/"},
		}},
		{Distro: "docker", Category: "app", URLs: []ISODownload{
			{Name: "Docker CE", URL: "https://download.docker.com/linux/ubuntu/dists/noble/pool/stable/amd64/"},
		}},
		{Distro: "noto-fonts", Category: "font", URLs: []ISODownload{
			{Name: "Noto Sans", URL: "https://github.com/google/fonts/raw/main/ofl/notosans/"},
		}},
		{Distro: "fira-code", Category: "font", URLs: []ISODownload{
			{Name: "Fira Code", URL: "https://github.com/tonsky/FiraCode/releases"},
		}},
	}
}

// ---------------------------------------------------------------------------
// benchmark server helper
// ---------------------------------------------------------------------------

func newBenchServer(mirrors []mirrors.Mirror, entries []index.Entry) (s *Server) {
	cfg := &mockServerConfig{
		site:    makeSite(),
		isoInfo: makeISOInfo(),
	}

	getter := &mockMirrorGetter{
		mirrors:  mirrors,
		cacheTTL: 60 * time.Second,
		age:      5 * time.Second,
	}

	provider := &mockIndexProvider{
		entries:  entries,
		cacheTTL: 60 * time.Second,
		age:      3 * time.Second,
	}

	resolver := &mockPathResolver{
		prefix: []byte("/mirror/repo/"),
		record: purl.Record{
			Host:   "node1",
			FQDN:   "mirrors.example.com",
			Prefix: "/mirror/repo/",
		},
	}

	s, _ = NewServer(cfg, getter, provider, resolver, zerolog.Nop())
	return
}

// renderResponse forces the stream-writer body to be written, then discards it.
func renderResponse(ctx *fasthttp.RequestCtx) {
	bw := bufio.NewWriter(io.Discard)
	ctx.Response.Write(bw)
	bw.Flush()
}

// ---------------------------------------------------------------------------
// benchmarks: mirrors page
// ---------------------------------------------------------------------------

func BenchmarkHandleMirrors(b *testing.B) {
	counts := []int{10, 50, 100, 500}
	for _, n := range counts {
		b.Run(fmt.Sprintf("mirrors=%d", n), func(b *testing.B) {
			b.StopTimer()
			srv := newBenchServer(makeMirrors(n), nil)
			b.StartTimer()

			for range b.N {
				ctx := &fasthttp.RequestCtx{}
				srv.HandleMirrors(ctx)
				renderResponse(ctx)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// benchmarks: status page
// ---------------------------------------------------------------------------

func BenchmarkHandleStatus(b *testing.B) {
	counts := []int{10, 50, 100, 500}
	for _, n := range counts {
		b.Run(fmt.Sprintf("mirrors=%d", n), func(b *testing.B) {
			b.StopTimer()
			srv := newBenchServer(makeMirrors(n), nil)
			b.StartTimer()

			for range b.N {
				ctx := &fasthttp.RequestCtx{}
				srv.HandleStatus(ctx)
				renderResponse(ctx)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// benchmarks: downloads index page
// ---------------------------------------------------------------------------

func BenchmarkHandleDownloads(b *testing.B) {
	b.StopTimer()
	srv := newBenchServer(nil, nil)
	b.StartTimer()

	for range b.N {
		ctx := &fasthttp.RequestCtx{}
		srv.HandleDownloads(ctx)
		renderResponse(ctx)
	}
}

// ---------------------------------------------------------------------------
// benchmarks: downloads detail page
// ---------------------------------------------------------------------------

func BenchmarkHandleDownloadsDetail(b *testing.B) {
	b.StopTimer()
	srv := newBenchServer(nil, nil)
	b.StartTimer()

	for range b.N {
		ctx := &fasthttp.RequestCtx{}
		ctx.SetUserValue("distro", "ubuntu")
		srv.HandleDownloadsDetail(ctx)
		renderResponse(ctx)
	}
}

// ---------------------------------------------------------------------------
// benchmarks: browse page
// ---------------------------------------------------------------------------

func BenchmarkHandleBrowse(b *testing.B) {
	counts := []int{10, 50, 100, 500}
	for _, n := range counts {
		b.Run(fmt.Sprintf("entries=%d", n), func(b *testing.B) {
			b.StopTimer()
			srv := newBenchServer(nil, makeEntries(n))
			b.StartTimer()

			for range b.N {
				ctx := &fasthttp.RequestCtx{}
				ctx.Request.SetRequestURI("/browse?path=/ubuntu/pool/main/")
				srv.HandleBrowse(ctx)
				renderResponse(ctx)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// benchmarks: 404 page
// ---------------------------------------------------------------------------

func BenchmarkHandleNotFound(b *testing.B) {
	b.StopTimer()
	srv := newBenchServer(nil, nil)
	b.StartTimer()

	for range b.N {
		ctx := &fasthttp.RequestCtx{}
		srv.handleNotFound(ctx, "", "")
		renderResponse(ctx)
	}
}
