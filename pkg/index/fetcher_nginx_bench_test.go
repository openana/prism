// LLM usage: generated with deepseek-v4-pro and modified manually.
package index

import (
	"context"
	"crypto/sha256"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// ---------- Benchmark Config ----------

// mockNginxBenchConfig implements NginxFetcherConfig for benchmarks.
// It is intentionally minimal — benchmarks don't need the extra TimeLayout
// field present in the test-only mock.
type mockNginxBenchConfig struct {
	baseURL string
	timeout time.Duration
}

func (m mockNginxBenchConfig) BaseURL() string        { return m.baseURL }
func (m mockNginxBenchConfig) Timeout() time.Duration { return m.timeout }
func (m mockNginxBenchConfig) IsFetcherConfig()       {}

// ---------- Mock Nginx Server ----------

const benchMaxEntries = 64

// newMockNginxServer creates an httptest.Server that mimics the behavior of
// test/mock-nginx-index/main.go: deterministic random Nginx-format JSON
// entries based on SHA256 of the request path.
//
// By default the 1/5 chance of HTTP 403 from the original mock is disabled
// so benchmarks get clean, reproducible measurements of the success path.
// Set forbidRate > 0 (e.g. 0.2) to re-enable it.
func newMockNginxServer(forbidRate float64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seed := sha256.Sum256([]byte(r.URL.Path))
		rnd := rand.New(rand.NewChaCha8(seed))

		if forbidRate > 0 && rnd.Float64() < forbidRate {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		n := rnd.IntN(benchMaxEntries)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if n == 0 {
			w.Write([]byte("[]\n"))
			return
		}

		w.Write([]byte("[\n"))
		writeNginxLine(w, rnd)
		for range n - 1 {
			w.Write([]byte(",\n"))
			writeNginxLine(w, rnd)
		}
		w.Write([]byte("\n]\n"))
	}))
}

var benchCharset = []byte("abcdefghijklmnopqrstuvwxyz0123456789-")

func benchFileName(rnd *rand.Rand) string {
	nameLen := 3 + rnd.IntN(20)
	buf := make([]byte, nameLen)
	for i := range buf {
		buf[i] = benchCharset[rnd.IntN(len(benchCharset))]
	}
	return string(buf)
}

var benchGMT = time.FixedZone("GMT", 0)

// benchMinUnix and benchMaxUnix define a Unix timestamp range that always
// produces RFC1123 strings parseable by time.Parse. The original mock uses
// unbounded rnd.Int64() which can produce timestamps whose formatted output
// exceeds RFC1123's 4-digit year constraint. For benchmarks we constrain
// to the range year ~2000 to ~2100 to get clean, reproducible measurements
// of the success path.
const (
	benchMinUnix = 946684800  // 2000-01-01
	benchMaxUnix = 4102444800 // 2100-01-01
)

func benchFormatTime(rnd *rand.Rand) string {
	ts := benchMinUnix + rnd.Int64N(benchMaxUnix-benchMinUnix)
	t := time.Unix(ts, 0).In(benchGMT)
	return t.Format(time.RFC1123)
}

func writeNginxLine(w http.ResponseWriter, rnd *rand.Rand) {
	name := benchFileName(rnd)
	mtime := benchFormatTime(rnd)
	switch rnd.IntN(3) {
	case 0:
		w.Write([]byte(`{ "name":"`))
		w.Write([]byte(name))
		w.Write([]byte(`", "type":"file", "mtime":"`))
		w.Write([]byte(mtime))
		w.Write([]byte(`", "size":`))
		w.Write([]byte(strconv.FormatInt(rnd.Int64(), 10)))
		w.Write([]byte(" }"))
	case 1:
		w.Write([]byte(`{ "name":"`))
		w.Write([]byte(name))
		w.Write([]byte(`", "type":"directory", "mtime":"`))
		w.Write([]byte(mtime))
		w.Write([]byte(`" }`))
	default:
		w.Write([]byte(`{ "name":"`))
		w.Write([]byte(name))
		w.Write([]byte(`", "type":"other", "mtime":"`))
		w.Write([]byte(mtime))
		w.Write([]byte(`" }`))
	}
}

// ---------- Full HTTP Round-Trip ----------

func BenchmarkNginxFetcher_AllOrErr(b *testing.B) {
	srv := newMockNginxServer(0)
	defer srv.Close()

	fetcher := NewNginxFetcher(mockNginxBenchConfig{
		baseURL: srv.URL + "/",
		timeout: 5 * time.Second,
	}, zerolog.Nop())

	path := []byte("/bench/")

	b.ResetTimer()
	for range b.N {
		it, err := fetcher.AllOrErr(context.Background(), path)
		if err != nil {
			b.Fatalf("AllOrErr failed: %v", err)
		}
		// Consume all entries.
		for range it {
		}
	}
}

// ---------- Concurrent Requests ----------

func BenchmarkNginxFetcher_Concurrent(b *testing.B) {
	srv := newMockNginxServer(0)
	defer srv.Close()

	fetcher := NewNginxFetcher(mockNginxBenchConfig{
		baseURL: srv.URL + "/",
		timeout: 5 * time.Second,
	}, zerolog.Nop())

	path := []byte("/bench/concurrent/")

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			it, err := fetcher.AllOrErr(context.Background(), path)
			if err != nil {
				b.Errorf("AllOrErr failed: %v", err)
				return
			}
			for range it {
			}
		}
	})
}
