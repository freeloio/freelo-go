// Package freelotime defines a time.Time wrapper that handles Freelo's
// two on-the-wire timestamp formats:
//
//   - RFC3339 with timezone, e.g. "2026-04-24T11:12:38+02:00" or "...Z".
//   - Local-time without timezone, e.g. "2026-04-24T11:12:38". Server-side
//     this represents wall-clock time in Europe/Prague.
//
// The generated client (freeloapi) uses freelotime.Time on every
// `format: date-time` field so typed *WithResponse methods decode out of
// the box. Consumers can also construct freelotime.Time values directly.
package freelotime

import (
	"errors"
	"fmt"
	"strings"
	"time"
	// time/tzdata embeds the IANA timezone database into the binary, so
	// time.LoadLocation("Europe/Prague") works on minimal images
	// (Alpine, distroless, scratch) that don't ship /usr/share/zoneinfo.
	// ~450 KB cost paid once at SDK level rather than asked of every
	// consumer.
	_ "time/tzdata"
)

// pragueLocation is the timezone Freelo's server-side timestamps are
// rendered in when no zone is included on the wire (the server reports
// "wall-clock Prague time", not UTC). Resolved once at init for parsing.
var pragueLocation *time.Location

func init() {
	loc, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		// time/tzdata is imported above, so this should never fail. Fall
		// back to a fixed +01:00 offset (CET, no DST) to avoid panicking;
		// at worst, summer-time timestamps drift by an hour rather than
		// crashing the SDK.
		loc = time.FixedZone("Europe/Prague", 1*60*60)
	}
	pragueLocation = loc
}

// localTimeLayout matches Freelo's no-timezone form, e.g. "2026-04-24T11:12:38".
const localTimeLayout = "2006-01-02T15:04:05"

// Time wraps time.Time with JSON (un)marshalling that tolerates Freelo's
// two timestamp shapes:
//
//   - RFC3339 with timezone, e.g. "2026-04-24T11:12:38+02:00" or "...Z".
//     Honored as-is; the result is normalized to UTC.
//
//   - Local-time without timezone, e.g. "2026-04-24T11:12:38". This is
//     what most Freelo endpoints emit today. Interpreted as Europe/Prague
//     wall-clock and converted to UTC.
//
// Marshalling always emits RFC3339 in UTC (e.g. "2026-04-24T09:12:38Z"),
// which the server accepts on the way back in.
//
// Use freelo.Time anywhere the generated client expects a date-time field;
// the spec preprocessor in scripts/specgen wires this in by injecting
// `x-go-type: freelo.Time` on every `format: date-time` schema before
// oapi-codegen runs.
type Time struct {
	time.Time
}

// UnmarshalJSON parses a JSON string per the rules documented on Time.
func (t *Time) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	s := strings.Trim(string(data), `"`)
	if s == "" || s == "null" {
		t.Time = time.Time{}
		return nil
	}

	// RFC3339 covers both "...Z" and "...+02:00" forms.
	if parsed, err := time.Parse(time.RFC3339, s); err == nil {
		t.Time = parsed.UTC()
		return nil
	}

	// Fallback: no zone on the wire → server emitted Prague wall-clock.
	if parsed, err := time.ParseInLocation(localTimeLayout, s, pragueLocation); err == nil {
		t.Time = parsed.UTC()
		return nil
	}

	return fmt.Errorf("freelo: cannot parse timestamp %q (expected RFC3339 or %q)", s, localTimeLayout)
}

// MarshalJSON emits the embedded time as RFC3339 in UTC. Zero values
// marshal to JSON null so the field can be safely omitempty-guarded by
// the generated client.
func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + t.UTC().Format(time.RFC3339) + `"`), nil
}

// String returns the RFC3339-UTC form, matching MarshalJSON without quotes.
func (t Time) String() string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// ErrInvalidTime is returned when UnmarshalJSON receives a string that
// matches neither RFC3339 nor the local-time layout. Wrapped in the
// returned error; check via errors.Is.
var ErrInvalidTime = errors.New("freelo: invalid timestamp format")
