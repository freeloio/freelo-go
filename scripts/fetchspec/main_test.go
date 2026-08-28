package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// specServer serves body with the given validators and records what the client
// sent, so the tests can assert on the conditional headers. A request carrying a
// matching If-None-Match / If-Modified-Since gets a 304.
type specServer struct {
	body         string
	etag         string
	lastModified string

	gotINM   string
	gotIMS   string
	requests int
}

func (s *specServer) start(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests++
		s.gotINM = r.Header.Get("If-None-Match")
		s.gotIMS = r.Header.Get("If-Modified-Since")

		if s.etag != "" {
			w.Header().Set("ETag", s.etag)
		}
		if s.lastModified != "" {
			w.Header().Set("Last-Modified", s.lastModified)
		}
		if (s.etag != "" && s.gotINM == s.etag) || (s.lastModified != "" && s.gotIMS == s.lastModified) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Content-Type", "text/yaml")
		_, _ = w.Write([]byte(s.body))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// paths returns fresh spec/meta paths inside a temp dir.
func paths(t *testing.T) (spec, meta string) {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "freelo-api.yaml"), filepath.Join(dir, ".meta.json")
}

// patch stands in for the `make gen` patch pipeline: it mutates the downloaded
// spec the way the sed rename and patchspec do, then seals the result.
func patch(t *testing.T, spec, meta string) {
	t.Helper()
	body, err := os.ReadFile(spec)
	if err != nil {
		t.Fatalf("read spec: %v", err)
	}
	if err := os.WriteFile(spec, append(body, []byte("\n# patched\n")...), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	if err := runSeal(spec, meta); err != nil {
		t.Fatalf("runSeal: %v", err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func TestFetchColdThenNotModified(t *testing.T) {
	srv := &specServer{body: "openapi: 3.0.3\n", etag: `"abc123"`, lastModified: "Wed, 26 Aug 2026 10:00:00 GMT"}
	url := srv.start(t)
	spec, meta := paths(t)

	// Cold cache: no validators to send, full body written.
	if err := runFetch(url, spec, meta, false); err != nil {
		t.Fatalf("cold runFetch: %v", err)
	}
	if srv.gotINM != "" || srv.gotIMS != "" {
		t.Errorf("cold run sent validators: If-None-Match=%q If-Modified-Since=%q", srv.gotINM, srv.gotIMS)
	}
	if got := readFile(t, spec); got != srv.body {
		t.Errorf("spec = %q, want %q", got, srv.body)
	}

	m, err := loadMeta(meta)
	if err != nil {
		t.Fatalf("loadMeta: %v", err)
	}
	if m.ETag != `"abc123"` || m.LastModified != "Wed, 26 Aug 2026 10:00:00 GMT" {
		t.Errorf("validators not recorded: %+v", m)
	}
	if m.PatchedSHA256 != "" {
		t.Errorf("patched_sha256 = %q before sealing, want empty", m.PatchedSHA256)
	}

	patch(t, spec, meta)
	patched := readFile(t, spec)

	// Warm cache: validators sent, 304 returned, patched spec left alone.
	if err := runFetch(url, spec, meta, false); err != nil {
		t.Fatalf("warm runFetch: %v", err)
	}
	if srv.gotINM != `"abc123"` {
		t.Errorf("If-None-Match = %q, want %q", srv.gotINM, `"abc123"`)
	}
	if srv.gotIMS != "Wed, 26 Aug 2026 10:00:00 GMT" {
		t.Errorf("If-Modified-Since = %q", srv.gotIMS)
	}
	if got := readFile(t, spec); got != patched {
		t.Errorf("304 overwrote the patched spec: %q", got)
	}
}

func TestFetchMetaStableAcrossRuns(t *testing.T) {
	// gen-check diffs the meta file, so an unchanged spec must not rewrite it.
	srv := &specServer{body: "openapi: 3.0.3\n", etag: `"abc123"`}
	url := srv.start(t)
	spec, meta := paths(t)

	if err := runFetch(url, spec, meta, false); err != nil {
		t.Fatalf("cold runFetch: %v", err)
	}
	patch(t, spec, meta)

	first := readFile(t, meta)
	for i := 0; i < 3; i++ {
		if err := runFetch(url, spec, meta, false); err != nil {
			t.Fatalf("runFetch %d: %v", i, err)
		}
		if err := runSeal(spec, meta); err != nil {
			t.Fatalf("runSeal %d: %v", i, err)
		}
		if got := readFile(t, meta); got != first {
			t.Fatalf("meta changed on run %d:\n got %s\nwant %s", i, got, first)
		}
	}
}

func TestFetchTamperedSpecIgnoresCache(t *testing.T) {
	// A hand-edited spec must not be able to hide behind a 304, otherwise
	// gen-check would green-light a spec upstream never served.
	srv := &specServer{body: "openapi: 3.0.3\n", etag: `"abc123"`}
	url := srv.start(t)
	spec, meta := paths(t)

	if err := runFetch(url, spec, meta, false); err != nil {
		t.Fatalf("cold runFetch: %v", err)
	}
	patch(t, spec, meta)

	if err := os.WriteFile(spec, []byte("openapi: 3.0.3\n# sneaky local edit\n"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}
	srv.gotINM = ""
	if err := runFetch(url, spec, meta, false); err != nil {
		t.Fatalf("runFetch after tamper: %v", err)
	}
	if srv.gotINM != "" {
		t.Errorf("sent If-None-Match %q for a tampered spec", srv.gotINM)
	}
	if got := readFile(t, spec); got != srv.body {
		t.Errorf("tampered spec not restored from upstream: %q", got)
	}
}

func TestFetchNoValidatorsFallsBackToHash(t *testing.T) {
	// Today's api.freelo.io behaviour: no ETag, no Last-Modified.
	srv := &specServer{body: "openapi: 3.0.3\n"}
	url := srv.start(t)
	spec, meta := paths(t)

	if err := runFetch(url, spec, meta, false); err != nil {
		t.Fatalf("cold runFetch: %v", err)
	}
	patch(t, spec, meta)
	patched := readFile(t, spec)

	// Same bytes upstream → recognised as unchanged, patched spec preserved.
	if err := runFetch(url, spec, meta, false); err != nil {
		t.Fatalf("warm runFetch: %v", err)
	}
	if got := readFile(t, spec); got != patched {
		t.Errorf("unchanged spec was overwritten: %q", got)
	}

	// Upstream changes → new body written, stale patched fingerprint cleared.
	srv.body = "openapi: 3.0.3\n# upstream change\n"
	if err := runFetch(url, spec, meta, false); err != nil {
		t.Fatalf("changed runFetch: %v", err)
	}
	if got := readFile(t, spec); got != srv.body {
		t.Errorf("spec = %q, want %q", got, srv.body)
	}
	m, err := loadMeta(meta)
	if err != nil {
		t.Fatalf("loadMeta: %v", err)
	}
	if m.PatchedSHA256 != "" {
		t.Errorf("patched_sha256 = %q after upstream change, want empty", m.PatchedSHA256)
	}
	if m.RawSHA256 != sha256Hex([]byte(srv.body)) {
		t.Errorf("raw_sha256 not updated: %+v", m)
	}
}

func TestFetchForceSkipsValidators(t *testing.T) {
	srv := &specServer{body: "openapi: 3.0.3\n", etag: `"abc123"`}
	url := srv.start(t)
	spec, meta := paths(t)

	if err := runFetch(url, spec, meta, false); err != nil {
		t.Fatalf("cold runFetch: %v", err)
	}
	patch(t, spec, meta)

	srv.gotINM = ""
	if err := runFetch(url, spec, meta, true); err != nil {
		t.Fatalf("forced runFetch: %v", err)
	}
	if srv.gotINM != "" {
		t.Errorf("-force still sent If-None-Match %q", srv.gotINM)
	}
	if got := readFile(t, spec); got != srv.body {
		t.Errorf("-force did not re-download: %q", got)
	}
}

func TestFetchURLChangeIgnoresCache(t *testing.T) {
	// Validators belong to one URL; reusing them against another would be wrong.
	first := &specServer{body: "openapi: 3.0.3\n", etag: `"abc123"`}
	firstURL := first.start(t)
	spec, meta := paths(t)

	if err := runFetch(firstURL, spec, meta, false); err != nil {
		t.Fatalf("cold runFetch: %v", err)
	}
	patch(t, spec, meta)

	second := &specServer{body: "openapi: 3.1.0\n", etag: `"abc123"`}
	secondURL := second.start(t)
	if err := runFetch(secondURL, spec, meta, false); err != nil {
		t.Fatalf("runFetch against new URL: %v", err)
	}
	if second.gotINM != "" {
		t.Errorf("reused If-None-Match %q against a different URL", second.gotINM)
	}
	if got := readFile(t, spec); got != second.body {
		t.Errorf("spec = %q, want %q", got, second.body)
	}
	m, err := loadMeta(meta)
	if err != nil {
		t.Fatalf("loadMeta: %v", err)
	}
	if m.URL != secondURL {
		t.Errorf("meta url = %q, want %q", m.URL, secondURL)
	}
}

func TestFetchRejectsBadResponses(t *testing.T) {
	spec, meta := paths(t)

	t.Run("empty body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		t.Cleanup(srv.Close)
		if err := runFetch(srv.URL, spec, meta, false); err == nil {
			t.Error("want error for an empty document, got nil")
		}
		if _, err := os.Stat(spec); !os.IsNotExist(err) {
			t.Error("empty document was written to disk")
		}
	})

	t.Run("server error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		t.Cleanup(srv.Close)
		if err := runFetch(srv.URL, spec, meta, false); err == nil {
			t.Error("want error for HTTP 500, got nil")
		}
	})

	t.Run("304 without a cached spec", func(t *testing.T) {
		// Cannot happen via runFetch (it only sends validators when the
		// fingerprint matches), but a proxy could still answer 304 unprompted.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotModified)
		}))
		t.Cleanup(srv.Close)
		if err := runFetch(srv.URL, spec, meta, false); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

func TestSealWithoutFetchFails(t *testing.T) {
	spec, meta := paths(t)
	if err := os.WriteFile(spec, []byte("openapi: 3.0.3\n"), 0o644); err != nil {
		t.Fatalf("write spec: %v", err)
	}
	if err := runSeal(spec, meta); err == nil {
		t.Error("want error when sealing without a recorded upstream fingerprint, got nil")
	}
}
