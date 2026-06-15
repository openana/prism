// LLM usage: generated with deepseek-v4-pro and modified manually.
package url

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

// stubConfig implements TrieResolverConfig for tests.
type stubConfig struct {
	records map[string]Record
}

func (c stubConfig) Records() map[string]Record {
	return c.records
}

// helpResolve builds a TrieResolver with the given records and a discard logger.
func helpResolve(records map[string]Record) *TrieResolver {
	return NewTrieResolver(stubConfig{records: records}, zerolog.New(io.Discard).Level(zerolog.Disabled))
}

// --- Phase 1: Tracer Bullet ---

func TestTrieResolver_Append_HappyPath(t *testing.T) {
	rt := helpResolve(map[string]Record{
		"/mirror/ubuntu/": {Host: "node1", FQDN: "mirrors.example.com", Prefix: "/mirror/ubuntu/"},
	})

	result, r, ok := rt.Append([]byte("/mirror/ubuntu/pool/main/x"), nil)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if string(result) != "/mirror/ubuntu/pool/main/x" {
		t.Errorf("result = %q, want %q", result, "/mirror/ubuntu/pool/main/x")
	}
	if r.Host != "node1" {
		t.Errorf("host = %q, want %q", r.Host, "node1")
	}
	if r.FQDN != "mirrors.example.com" {
		t.Errorf("fqdn = %q, want %q", r.FQDN, "mirrors.example.com")
	}
}

// --- Phase 2: Happy Path Extensions ---

func TestTrieResolver_Append_LongestPrefixMatch(t *testing.T) {
	rt := helpResolve(map[string]Record{
		"/mirror/":        {Host: "generic", FQDN: "generic.example.com", Prefix: "/mirror/"},
		"/mirror/ubuntu/": {Host: "ubuntu", FQDN: "ubuntu.example.com", Prefix: "/mirror/ubuntu/"},
	})

	cases := []struct {
		path   string
		wantOK bool
		wantR  string
		wantH  string
	}{
		{"/mirror/ubuntu/pool/main/x", true, "/mirror/ubuntu/pool/main/x", "ubuntu"},
		{"/mirror/centos/7/x", true, "/mirror/centos/7/x", "generic"},
		{"/mirror/", true, "/mirror/", "generic"},
		{"/mirror", false, "", ""},
	}

	for _, c := range cases {
		result, r, ok := rt.Append([]byte(c.path), nil)
		if ok != c.wantOK {
			t.Errorf("Append(%q) ok=%v, want %v", c.path, ok, c.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if string(result) != c.wantR {
			t.Errorf("Append(%q) result=%q, want %q", c.path, result, c.wantR)
		}
		if r.Host != c.wantH {
			t.Errorf("Append(%q) host=%q, want %q", c.path, r.Host, c.wantH)
		}
	}
}

func TestTrieResolver_Append_EmptyPrefix(t *testing.T) {
	rt := helpResolve(map[string]Record{
		"": {Host: "catch-all", FQDN: "default.example.com", Prefix: ""},
	})

	cases := []struct {
		path string
	}{
		{"/any/path"},
		{"/"},
		{""},
		{"alpine"},
	}

	for _, c := range cases {
		result, r, ok := rt.Append([]byte(c.path), nil)
		if !ok {
			t.Errorf("Append(%q) ok=false, want true", c.path)
			continue
		}
		// Empty prefix means the result is just the original path appended after ""
		if string(result) != c.path {
			t.Errorf("Append(%q) result=%q, want %q", c.path, result, c.path)
		}
		if r.Host != "catch-all" {
			t.Errorf("Append(%q) host=%q, want %q", c.path, r.Host, "catch-all")
		}
	}
}

func TestTrieResolver_Append_WithDst(t *testing.T) {
	rt := helpResolve(map[string]Record{
		"/mirror/ubuntu/": {Host: "node1", FQDN: "mirrors.example.com", Prefix: "/mirror/ubuntu/"},
	})

	dst := []byte("prefix:")
	result, _, ok := rt.Append([]byte("/mirror/ubuntu/pool/main/x"), dst)
	if !ok {
		t.Fatal("expected ok=true")
	}
	want := "prefix:/mirror/ubuntu/pool/main/x"
	if string(result) != want {
		t.Errorf("result = %q, want %q", result, want)
	}

	// Verify dst was not mutated; result is a new slice appended to dst
	if string(dst) != "prefix:" {
		t.Errorf("dst was mutated: %q", dst)
	}
}

// --- Phase 3: Not Resolved ---

func TestTrieResolver_Append_NoMatch(t *testing.T) {
	rt := helpResolve(map[string]Record{
		"/mirror/ubuntu/": {Host: "node1", FQDN: "mirrors.example.com", Prefix: "/mirror/ubuntu/"},
	})

	cases := []struct {
		path string
	}{
		{"/other/path"},
		{"/mirror/ubuntu"},     // no trailing slash, so no match
		{"/mirror/ubuntuxxx/"}, // partial byte match but different string
		{"ubuntu/"},            // missing leading slash
	}

	for _, c := range cases {
		_, _, ok := rt.Append([]byte(c.path), nil)
		if ok {
			t.Errorf("Append(%q) ok=true, want false", c.path)
		}
	}
}

func TestTrieResolver_Append_EmptyTrie(t *testing.T) {
	rt := helpResolve(map[string]Record{})

	_, _, ok := rt.Append([]byte("/anything"), nil)
	if ok {
		t.Error("Append with empty trie: ok=true, want false")
	}
}

func TestTrieResolver_Append_AfterDelRecord(t *testing.T) {
	rt := helpResolve(map[string]Record{
		"/mirror/ubuntu/": {Host: "node1", FQDN: "mirrors.example.com", Prefix: "/mirror/ubuntu/"},
	})

	// Initial: should match
	_, _, ok := rt.Append([]byte("/mirror/ubuntu/pool/main/x"), nil)
	if !ok {
		t.Fatal("expected initial match")
	}

	// Delete and commit
	found := rt.DelRecord("/mirror/ubuntu/")
	if !found {
		t.Fatal("DelRecord should have found the prefix")
	}
	rt.Commit()

	// After deletion: should not match
	_, _, ok = rt.Append([]byte("/mirror/ubuntu/pool/main/x"), nil)
	if ok {
		t.Error("Append after DelRecord: ok=true, want false")
	}
}

// --- Phase 4: Mutation Methods ---

func TestTrieResolver_SetRecord(t *testing.T) {
	rt := helpResolve(map[string]Record{})

	// Set a new record
	rt.SetRecord("/new/prefix/", Record{Host: "h1", FQDN: "f1.example.com", Prefix: "/new/prefix/"})

	// HasRecord should see it immediately (before Commit)
	if !rt.HasRecord("/new/prefix/") {
		t.Error("HasRecord after SetRecord: got false, want true")
	}

	// Append should NOT see it before Commit (atomic swap hasn't happened)
	_, _, ok := rt.Append([]byte("/new/prefix/sub"), nil)
	if ok {
		t.Error("Append before Commit: ok=true, want false (trie not yet rebuilt)")
	}

	// Commit and verify Append sees it
	rt.Commit()
	result, r, ok := rt.Append([]byte("/new/prefix/sub"), nil)
	if !ok {
		t.Fatal("Append after Commit: ok=false, want true")
	}
	if string(result) != "/new/prefix/sub" {
		t.Errorf("result = %q, want %q", result, "/new/prefix/sub")
	}
	if r.Host != "h1" {
		t.Errorf("host = %q, want %q", r.Host, "h1")
	}
}

func TestTrieResolver_DelRecord(t *testing.T) {
	rt := helpResolve(map[string]Record{
		"/existing/": {Host: "h", FQDN: "f.example.com", Prefix: "/existing/"},
	})

	// Delete existing
	found := rt.DelRecord("/existing/")
	if !found {
		t.Error("DelRecord existing: got false, want true")
	}

	// Delete already-deleted
	found = rt.DelRecord("/existing/")
	if found {
		t.Error("DelRecord already-deleted: got true, want false")
	}

	// Delete never-existed
	found = rt.DelRecord("/never-there/")
	if found {
		t.Error("DelRecord non-existent: got true, want false")
	}
}

func TestTrieResolver_HasRecord(t *testing.T) {
	rt := helpResolve(map[string]Record{
		"/existing/": {Host: "h", FQDN: "f.example.com", Prefix: "/existing/"},
	})

	if !rt.HasRecord("/existing/") {
		t.Error("HasRecord existing: got false, want true")
	}

	if rt.HasRecord("/nope/") {
		t.Error("HasRecord non-existent: got true, want false")
	}
}

func TestTrieResolver_Commit_Visibility(t *testing.T) {
	rt := helpResolve(map[string]Record{
		"/v1/": {Host: "v1", FQDN: "v1.example.com", Prefix: "/v1/"},
	})

	// Baseline: v1 is visible
	_, r, ok := rt.Append([]byte("/v1/sub"), nil)
	if !ok || r.Host != "v1" {
		t.Fatal("baseline: expected v1 to match")
	}

	// Set v2 but don't commit
	rt.SetRecord("/v2/", Record{Host: "v2", FQDN: "v2.example.com", Prefix: "/v2/"})

	// v2 should NOT be visible yet (trie not rebuilt)
	_, _, ok = rt.Append([]byte("/v2/sub"), nil)
	if ok {
		t.Error("v2 visible before Commit: expected false")
	}

	// v1 should still work (old trie still in place)
	_, r, ok = rt.Append([]byte("/v1/sub"), nil)
	if !ok || r.Host != "v1" {
		t.Error("v1 should still resolve via old trie")
	}

	// Commit and verify both are visible
	rt.Commit()
	_, r, ok = rt.Append([]byte("/v1/sub"), nil)
	if !ok || r.Host != "v1" {
		t.Error("v1 not visible after Commit")
	}
	_, r, ok = rt.Append([]byte("/v2/sub"), nil)
	if !ok || r.Host != "v2" {
		t.Error("v2 not visible after Commit")
	}
}

// --- Phase 5: Concurrent Commits ---

func TestTrieResolver_ConcurrentAppendAndCommit(t *testing.T) {
	rt := helpResolve(map[string]Record{
		"/base/": {Host: "base", FQDN: "base.example.com", Prefix: "/base/"},
	})

	const (
		numAppenders = 10
		iterations   = 200
	)

	var wg sync.WaitGroup

	// Start appender goroutines that constantly call Append
	for i := 0; i < numAppenders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// Mix of paths that may or may not match depending on current trie state
				rt.Append([]byte("/base/sub/path"), nil)
				rt.Append([]byte("/dynamic/path"), nil)
				rt.Append([]byte("/base/"), nil)
			}
		}(i)
	}

	// Main goroutine: repeatedly add new records and commit
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			prefix := "/dynamic/" + string(rune('a'+i%26)) + "/"
			rt.SetRecord(prefix, Record{Host: "dyn", FQDN: "dyn.example.com", Prefix: prefix})
			rt.Commit()
			time.Sleep(time.Microsecond)
		}
		close(done)
	}()

	wg.Wait()
	<-done

	// Verify final state is sane: at least /base/ still resolves
	_, _, ok := rt.Append([]byte("/base/sub/path"), nil)
	if !ok {
		t.Error("/base/ should always resolve")
	}

	// Verify a dynamic prefix from the last iteration is visible after commit
	rt.Commit() // ensure final state
	lastPrefix := "/dynamic/z/"
	if !rt.HasRecord(lastPrefix) {
		// It's possible the last iteration didn't add 'z', but at least one should be there
		found := false
		for c := 'a'; c <= 'z'; c++ {
			if rt.HasRecord("/dynamic/" + string(c) + "/") {
				found = true
				break
			}
		}
		if !found {
			t.Error("no dynamic prefixes found after concurrent commits")
		}
	}
}

func TestTrieResolver_ConcurrentAppendAndDelRecord(t *testing.T) {
	rt := helpResolve(map[string]Record{
		"/base/":  {Host: "base", FQDN: "base.example.com", Prefix: "/base/"},
		"/todel/": {Host: "del", FQDN: "del.example.com", Prefix: "/todel/"},
		"/keep/":  {Host: "keep", FQDN: "keep.example.com", Prefix: "/keep/"},
	})

	const (
		numAppenders = 10
		iterations   = 200
	)

	var wg sync.WaitGroup

	// Start appender goroutines
	for i := 0; i < numAppenders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				rt.Append([]byte("/base/sub"), nil)
				rt.Append([]byte("/todel/sub"), nil)
				rt.Append([]byte("/keep/sub"), nil)
			}
		}(i)
	}

	// Main goroutine: add and delete records, committing each time
	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			// Re-add /todel/ then delete it again — toggling
			rt.SetRecord("/todel/", Record{Host: "del", FQDN: "del.example.com", Prefix: "/todel/"})
			rt.Commit()
			rt.DelRecord("/todel/")
			rt.Commit()
			time.Sleep(time.Microsecond)
		}
		close(done)
	}()

	wg.Wait()
	<-done

	// /base/ and /keep/ should always resolve (never deleted)
	_, _, ok := rt.Append([]byte("/base/sub"), nil)
	if !ok {
		t.Error("/base/ should always resolve")
	}
	_, _, ok = rt.Append([]byte("/keep/sub"), nil)
	if !ok {
		t.Error("/keep/ should always resolve")
	}
}
