package freelotime

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTime_UnmarshalJSON(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		wantUTC time.Time
		wantErr bool
	}{
		{
			name:    "RFC3339 with Z",
			input:   `"2026-04-24T11:12:38Z"`,
			wantUTC: time.Date(2026, 4, 24, 11, 12, 38, 0, time.UTC),
		},
		{
			name:    "RFC3339 with +02:00 offset",
			input:   `"2026-04-24T11:12:38+02:00"`,
			wantUTC: time.Date(2026, 4, 24, 9, 12, 38, 0, time.UTC),
		},
		{
			name:  "no zone, summer time → Prague is UTC+02:00",
			input: `"2026-07-15T14:30:00"`,
			// 14:30 Prague summer = 12:30 UTC.
			wantUTC: time.Date(2026, 7, 15, 12, 30, 0, 0, time.UTC),
		},
		{
			name:  "no zone, winter time → Prague is UTC+01:00",
			input: `"2026-01-15T14:30:00"`,
			// 14:30 Prague winter = 13:30 UTC.
			wantUTC: time.Date(2026, 1, 15, 13, 30, 0, 0, time.UTC),
		},
		{
			name:    "JSON null",
			input:   `null`,
			wantUTC: time.Time{},
		},
		{
			name:    "empty string",
			input:   `""`,
			wantUTC: time.Time{},
		},
		{
			name:    "garbage string",
			input:   `"not a timestamp"`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var got Time
			err := got.UnmarshalJSON([]byte(tc.input))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if !got.UTC().Equal(tc.wantUTC) {
				t.Fatalf("got %v, want %v", got.UTC(), tc.wantUTC)
			}
		})
	}
}

func TestTime_MarshalJSON(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   Time
		want string
	}{
		{
			name: "zero value → null",
			in:   Time{},
			want: "null",
		},
		{
			name: "UTC value",
			in:   Time{time.Date(2026, 4, 24, 11, 12, 38, 0, time.UTC)},
			want: `"2026-04-24T11:12:38Z"`,
		},
		{
			name: "value in non-UTC zone is normalized to UTC",
			// 11:12:38 Prague summer = 09:12:38 UTC.
			in:   Time{time.Date(2026, 7, 15, 11, 12, 38, 0, pragueLocation)},
			want: `"2026-07-15T09:12:38Z"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.in.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

// Round-trip via encoding/json the way the generated client does it.
func TestTime_RoundTripViaJSON(t *testing.T) {
	t.Parallel()

	type payload struct {
		Created Time `json:"created"`
	}

	// Server emits no-zone Prague form; client unmarshals; client marshals back.
	body := `{"created":"2026-04-24T11:12:38"}`

	var p payload
	if err := json.Unmarshal([]byte(body), &p); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	out, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// April 24 → Prague summer time (CEST = UTC+02:00) → 11:12:38 → 09:12:38 UTC.
	want := `{"created":"2026-04-24T09:12:38Z"}`
	if string(out) != want {
		t.Fatalf("round-trip got %s, want %s", out, want)
	}
}
