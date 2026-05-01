package freelo

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestWithBaseURL_RequiresHTTPS(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"https ok", "https://api.freelo.io/v1", false},
		{"http rejected", "http://api.freelo.io/v1", true},
		{"missing scheme rejected", "api.freelo.io/v1", true},
		{"empty rejected", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := defaultConfig()
			err := WithBaseURL(tc.input)(cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestWithBaseURL_TrimsTrailingSlash(t *testing.T) {
	t.Parallel()
	cfg := defaultConfig()
	if err := WithBaseURL("https://api.freelo.io/v1/")(cfg); err != nil {
		t.Fatalf("WithBaseURL: %v", err)
	}
	if cfg.baseURL != "https://api.freelo.io/v1" {
		t.Errorf("baseURL=%q, want trailing slash trimmed", cfg.baseURL)
	}
}

func TestWithUserAgent_RejectsEmpty(t *testing.T) {
	t.Parallel()
	cfg := defaultConfig()
	if err := WithUserAgent("")(cfg); err == nil {
		t.Error("WithUserAgent(\"\") returned nil error")
	}
	if err := WithUserAgent("   ")(cfg); err == nil {
		t.Error("WithUserAgent whitespace-only returned nil error")
	}
}

func TestWithRateLimit_AcceptsZero(t *testing.T) {
	t.Parallel()
	cfg := defaultConfig()
	if err := WithRateLimit(0)(cfg); err != nil {
		t.Fatalf("WithRateLimit(0) err=%v", err)
	}
	if cfg.rateInterval != 0 {
		t.Errorf("rateInterval=%v, want 0", cfg.rateInterval)
	}
}

func TestWithRateLimit_RejectsNegative(t *testing.T) {
	t.Parallel()
	cfg := defaultConfig()
	if err := WithRateLimit(-1 * time.Second)(cfg); err == nil {
		t.Error("WithRateLimit(-1s) returned nil error")
	}
}

func TestWithRetry_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		attempts int
		base     time.Duration
		max      time.Duration
		wantErr  bool
	}{
		{"ok", 3, 500 * time.Millisecond, 8 * time.Second, false},
		{"single attempt ok", 1, time.Millisecond, time.Second, false},
		{"zero attempts rejected", 0, time.Millisecond, time.Second, true},
		{"negative attempts rejected", -1, time.Millisecond, time.Second, true},
		{"max < base rejected", 3, time.Second, time.Millisecond, true},
		{"zero base rejected", 3, 0, time.Second, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := defaultConfig()
			err := WithRetry(tc.attempts, tc.base, tc.max)(cfg)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestWithHTTPClient_RejectsNil(t *testing.T) {
	t.Parallel()
	cfg := defaultConfig()
	if err := WithHTTPClient(nil)(cfg); err == nil {
		t.Error("WithHTTPClient(nil) returned nil error")
	}
}

func TestWithHTTPClient_Stores(t *testing.T) {
	t.Parallel()
	hc := &http.Client{Timeout: time.Second}
	cfg := defaultConfig()
	if err := WithHTTPClient(hc)(cfg); err != nil {
		t.Fatalf("WithHTTPClient: %v", err)
	}
	if cfg.httpClient != hc {
		t.Error("httpClient not stored")
	}
}

func TestWithRequestEditor_Appends(t *testing.T) {
	t.Parallel()
	cfg := defaultConfig()

	noop := RequestEditorFn(func(_ context.Context, _ *http.Request) error { return nil })
	if err := WithRequestEditor(noop)(cfg); err != nil {
		t.Fatalf("WithRequestEditor: %v", err)
	}
	if err := WithRequestEditor(noop)(cfg); err != nil {
		t.Fatalf("WithRequestEditor (2nd): %v", err)
	}
	if len(cfg.editors) != 2 {
		t.Errorf("editors=%d, want 2", len(cfg.editors))
	}
}

func TestWithRequestEditor_RejectsNil(t *testing.T) {
	t.Parallel()
	cfg := defaultConfig()
	if err := WithRequestEditor(nil)(cfg); err == nil {
		t.Error("WithRequestEditor(nil) returned nil error")
	}
}
