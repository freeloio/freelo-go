// Command patchgen rewrites the oapi-codegen output so that every
// `time.Time` field becomes a `freelotime.Time` field.
//
// Why this exists: Freelo's API emits timestamps without a timezone suffix
// ("2026-04-24T11:12:38"), which Go's time package can't unmarshal as
// time.RFC3339. The default generated typed client therefore fails on
// every endpoint that returns a date — including listing projects, tasks,
// comments, etc. freelotime.Time accepts both RFC3339-with-zone and the
// no-zone form (interpreted as Europe/Prague), so swapping the type makes
// the typed *WithResponse methods work out of the box.
//
// We do this as a post-generation patch (rather than a spec preprocessor
// or oapi-codegen import-mapping) because:
//   - The spec stays byte-identical to what Freelo publishes — `make gen`
//     tracks the upstream document with no Go-specific extensions baked in.
//   - oapi-codegen v2 doesn't reliably honor x-go-type on primitive
//     `format: date-time` schemas (it does for named schemas).
//   - A regex substitution over the generated file is simple, transparent,
//     and easy to audit on every regen.
//
// Run via `make gen`. Idempotent — running it twice on the same input is
// a no-op.
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

const (
	genFile        = "freeloapi/freeloapi.gen.go"
	freelotimePath = "github.com/freeloio/freelo-go/freelotime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "patchgen:", err)
		os.Exit(1)
	}
}

func run() error {
	src, err := os.ReadFile(genFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", genFile, err)
	}

	out := string(src)

	// 1. Replace all *time.Time field types with *freelotime.Time.
	//    Boundary check ensures we don't touch unrelated identifiers.
	out = regexp.MustCompile(`\*time\.Time\b`).ReplaceAllString(out, "*freelotime.Time")

	// 2. Replace bare time.Time field types with freelotime.Time. Match
	//    only when preceded by whitespace (struct field declaration) and
	//    followed by whitespace, end-of-line, or backtick (struct tag).
	out = regexp.MustCompile(`(\s)time\.Time\b`).ReplaceAllString(out, `${1}freelotime.Time`)

	// 3. Swap the time import for freelotime. The generated file imports
	//    "time" on its own line; replace that exact line.
	timeImport := `	"time"`
	freelotimeImport := `	freelotime "` + freelotimePath + `"`
	if !strings.Contains(out, timeImport) && !strings.Contains(out, freelotimeImport) {
		return fmt.Errorf("expected import line %q not found in %s", strings.TrimSpace(timeImport), genFile)
	}
	if strings.Contains(out, timeImport) {
		out = strings.Replace(out, timeImport, freelotimeImport, 1)
	}

	// 4. Sanity check: no time.Time references should remain. (time.X for
	//    other X — there are none in the generated file today — would slip
	//    past, which is fine; we only care about time.Time here.)
	if strings.Contains(out, "time.Time") {
		return fmt.Errorf("internal: time.Time still present after patch (left-over occurrence?)")
	}

	if err := os.WriteFile(genFile, []byte(out), 0644); err != nil {
		return fmt.Errorf("write %s: %w", genFile, err)
	}

	fmt.Println("patchgen: rewrote time.Time → freelotime.Time in", genFile)
	return nil
}
