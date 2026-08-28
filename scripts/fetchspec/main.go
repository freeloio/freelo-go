// Command fetchspec downloads the Freelo OpenAPI document with a conditional
// GET, replacing the plain `curl` call that used to open `make gen`.
//
// Why this exists: `make gen-check` runs on every CI push and on the weekly
// spec refresh, and each run re-transferred the full ~270 KB document even when
// upstream had not changed. A conditional request lets the server answer "304
// Not Modified" instead, and gives us a recorded fingerprint of the exact
// upstream bytes the committed spec was derived from.
//
// Cache metadata lives next to the spec in a small JSON file that is committed
// to the repo — CI checks out a fresh tree, so an uncommitted cache would never
// produce a 304 there. The file therefore holds only stable fields (no
// timestamps): an unchanged spec must serialize byte-identically, otherwise
// `gen-check`'s `git diff` would report drift on every run.
//
// Three fields, three jobs:
//
//	etag / last_modified  validators echoed back as If-None-Match /
//	                      If-Modified-Since to earn the 304.
//	raw_sha256            hash of the upstream body, so a server that sends no
//	                      validators (see below) can still be told "unchanged".
//	patched_sha256        hash of spec/freelo-api.yaml *after* the patch
//	                      pipeline, recorded by -seal. Guards the shortcut: a
//	                      hand-edited spec must not be able to hide behind a
//	                      304, so a fingerprint mismatch drops the validators
//	                      and forces a full re-download.
//
// As of this writing api.freelo.io sends neither ETag nor Last-Modified for the
// spec (it replies `cache-control: no-cache, private`), so every run currently
// takes the 200 path and falls back to comparing raw_sha256 — the same work the
// old curl did, plus a definitive changed/unchanged verdict. The conditional
// headers start paying off, with no further change here, the day upstream emits
// a validator.
//
// Usage (see the `gen` target):
//
//	go run ./scripts/fetchspec -url URL -out spec.yaml -meta meta.json
//	go run ./scripts/fetchspec -seal  -out spec.yaml -meta meta.json
//
// -force skips the validators for one run and always rewrites the spec from the
// response body. Exit status is 0 for both "changed" and "unchanged"; only a
// genuine failure is non-zero.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// userAgent identifies this tool to api.freelo.io, which requires a User-Agent
// on API requests and logs it for the docs endpoint too.
const userAgent = "freelo-go-fetchspec (https://github.com/freeloio/freelo-go)"

const httpTimeout = 60 * time.Second

// meta is the committed cache state. Field order is the serialization order;
// keep it stable so unchanged runs leave the file untouched.
type meta struct {
	URL           string `json:"url"`
	ETag          string `json:"etag,omitempty"`
	LastModified  string `json:"last_modified,omitempty"`
	RawSHA256     string `json:"raw_sha256,omitempty"`
	PatchedSHA256 string `json:"patched_sha256,omitempty"`
}

func main() {
	var (
		url      = flag.String("url", "https://api.freelo.io/docs/v1/freelo-api.yaml", "spec URL")
		out      = flag.String("out", "spec/freelo-api.yaml", "path to write the spec to")
		metaPath = flag.String("meta", "spec/.freelo-api.meta.json", "path to the cache metadata file")
		force    = flag.Bool("force", false, "ignore cached validators and download unconditionally")
		seal     = flag.Bool("seal", false, "record the fingerprint of the patched spec and exit")
	)
	flag.Parse()

	var err error
	if *seal {
		err = runSeal(*out, *metaPath)
	} else {
		err = runFetch(*url, *out, *metaPath, *force)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "fetchspec:", err)
		os.Exit(1)
	}
}

// runFetch performs the conditional download and writes the spec when the body
// differs from what is already on disk.
func runFetch(url, out, metaPath string, force bool) error {
	m, err := loadMeta(metaPath)
	if err != nil {
		return err
	}

	// Validators only describe the upstream bytes the committed spec came from.
	// They are worthless — worse, actively misleading — if that spec is missing,
	// was hand-edited, or was fetched from a different URL.
	conditional := !force && m.URL == url && specMatchesFingerprint(out, m)
	if force {
		fmt.Println("fetchspec: -force given, skipping conditional headers")
	} else if !conditional && m.ETag+m.LastModified+m.RawSHA256 != "" {
		fmt.Println("fetchspec: local spec does not match recorded fingerprint, ignoring cache")
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "text/yaml, application/yaml, application/x-yaml, */*")
	if conditional {
		if m.ETag != "" {
			req.Header.Set("If-None-Match", m.ETag)
		}
		if m.LastModified != "" {
			req.Header.Set("If-Modified-Since", m.LastModified)
		}
	}

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotModified:
		// Nothing to write: the on-disk spec was derived from these exact bytes,
		// and specMatchesFingerprint just proved it is still intact.
		fmt.Printf("fetchspec: spec unchanged (HTTP 304, %s)\n", validatorSummary(m))
		return nil

	case http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
		if len(body) == 0 {
			return errors.New("upstream returned an empty document")
		}

		next := meta{
			URL:          url,
			ETag:         resp.Header.Get("ETag"),
			LastModified: resp.Header.Get("Last-Modified"),
			RawSHA256:    sha256Hex(body),
			// Carried over; -seal replaces it once the patches have run.
			PatchedSHA256: m.PatchedSHA256,
		}

		// The fallback for a server that offers no validators: identical bytes
		// mean the patch pipeline would reproduce the committed spec exactly, so
		// leave the file alone. -force opts out of every shortcut — it exists to
		// rebuild the spec from whatever upstream just sent.
		unchanged := !force && m.RawSHA256 != "" && m.RawSHA256 == next.RawSHA256 && specMatchesFingerprint(out, m)
		if unchanged {
			fmt.Printf("fetchspec: spec unchanged (HTTP 200, identical sha256 %s)\n", short(next.RawSHA256))
		} else {
			if err := writeFile(out, body); err != nil {
				return err
			}
			fmt.Printf("fetchspec: spec updated (%d bytes, sha256 %s)\n", len(body), short(next.RawSHA256))
			// The patched fingerprint describes the *previous* body; drop it so a
			// crash between here and -seal cannot leave a stale hash behind.
			next.PatchedSHA256 = ""
		}
		if next.ETag == "" && next.LastModified == "" {
			fmt.Println("fetchspec: upstream sent no ETag/Last-Modified — next run falls back to sha256 comparison")
		}
		return saveMeta(metaPath, next)

	default:
		return fmt.Errorf("GET %s: unexpected status %s", url, resp.Status)
	}
}

// runSeal records the hash of the spec as it stands after the patch pipeline,
// which is what specMatchesFingerprint validates on the next run.
func runSeal(out, metaPath string) error {
	m, err := loadMeta(metaPath)
	if err != nil {
		return err
	}
	if m.RawSHA256 == "" {
		return fmt.Errorf("%s holds no upstream fingerprint — run fetchspec without -seal first", metaPath)
	}
	sum, err := fileSHA256(out)
	if err != nil {
		return err
	}
	if m.PatchedSHA256 == sum {
		return nil // already sealed by an earlier run; keep the file untouched
	}
	m.PatchedSHA256 = sum
	if err := saveMeta(metaPath, m); err != nil {
		return err
	}
	fmt.Printf("fetchspec: sealed patched spec (sha256 %s)\n", short(sum))
	return nil
}

// specMatchesFingerprint reports whether the on-disk spec is byte-for-byte the
// patched file the recorded validators belong to.
func specMatchesFingerprint(out string, m meta) bool {
	if m.PatchedSHA256 == "" {
		return false
	}
	sum, err := fileSHA256(out)
	return err == nil && sum == m.PatchedSHA256
}

func loadMeta(path string) (meta, error) {
	var m meta
	buf, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil // first run
	}
	if err != nil {
		return m, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(buf, &m); err != nil {
		return meta{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return m, nil
}

func saveMeta(path string, m meta) error {
	buf, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	return writeFile(path, append(buf, '\n'))
}

func writeFile(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return sha256Hex(buf), nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}

// validatorSummary names the header that earned a 304, for the log line.
func validatorSummary(m meta) string {
	switch {
	case m.ETag != "" && m.LastModified != "":
		return "matched ETag + Last-Modified"
	case m.ETag != "":
		return "matched ETag " + m.ETag
	case m.LastModified != "":
		return "matched Last-Modified " + m.LastModified
	default:
		return "no validators sent"
	}
}
