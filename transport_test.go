package freelo

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/freeloio/freelo-go/auth"
)

// newDoerForTest returns a retryingDoer with the rate limiter disabled so
// tests finish in milliseconds. Production traffic uses DefaultRateInterval.
func newDoerForTest(httpClient *http.Client) *retryingDoer {
	return newRetryingDoer(&config{
		httpClient:    httpClient,
		rateInterval:  0,
		retryAttempts: DefaultMaxAttempts,
		backoffBase:   DefaultBackoffBase,
		backoffMax:    DefaultBackoffMax,
	})
}

func TestRetriesOn429ThenSucceeds(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	d := newDoerForTest(server.Client())
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := d.Do(req)
	if err != nil {
		t.Fatalf("Do err=%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status=%d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("server calls=%d, want 2 (1 retry)", got)
	}
}

func TestRetriesOn500ThenGivesUp(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	d := newDoerForTest(server.Client())
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := d.Do(req)
	if err != nil {
		t.Fatalf("Do err=%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status=%d, want 500 (last response after max attempts)", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != int32(DefaultMaxAttempts) {
		t.Errorf("server calls=%d, want %d", got, DefaultMaxAttempts)
	}
}

func TestDoesNotRetry4xxOtherThan429(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 422} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var calls int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				atomic.AddInt32(&calls, 1)
				w.WriteHeader(status)
			}))
			defer server.Close()

			d := newDoerForTest(server.Client())
			req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
			resp, err := d.Do(req)
			if err != nil {
				t.Fatalf("Do err=%v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != status {
				t.Errorf("status=%d, want %d", resp.StatusCode, status)
			}
			if got := atomic.LoadInt32(&calls); got != 1 {
				t.Errorf("server calls=%d, want 1 (non-retryable)", got)
			}
		})
	}
}

func TestReplayPOSTBodyOnRetry(t *testing.T) {
	var calls int32
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, string(body))
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := newDoerForTest(server.Client())
	payload := `{"name":"new task"}`
	req, _ := http.NewRequest(http.MethodPost, server.URL, strings.NewReader(payload))
	resp, err := d.Do(req)
	if err != nil {
		t.Fatalf("Do err=%v", err)
	}
	defer resp.Body.Close()

	if len(seen) != 2 {
		t.Fatalf("server saw %d bodies, want 2", len(seen))
	}
	if seen[0] != payload || seen[1] != payload {
		t.Errorf("body not replayed identically:\n  first=%q\n  second=%q", seen[0], seen[1])
	}
}

func TestRateLimitDelaysSecondRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := newRetryingDoer(&config{
		httpClient:    server.Client(),
		rateInterval:  100 * time.Millisecond,
		retryAttempts: DefaultMaxAttempts,
		backoffBase:   DefaultBackoffBase,
		backoffMax:    DefaultBackoffMax,
	})

	req1, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	req2, _ := http.NewRequest(http.MethodGet, server.URL, nil)

	start := time.Now()
	_, _ = d.Do(req1)
	_, _ = d.Do(req2)
	elapsed := time.Since(start)

	if elapsed < 90*time.Millisecond {
		t.Errorf("two requests in %v, want >= 100ms (rate limiter regressed?)", elapsed)
	}
}

func TestRetryAfterHeaderHonored(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	d := newDoerForTest(server.Client())
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := d.Do(req)
	if err != nil {
		t.Fatalf("Do err=%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status=%d, want 200", resp.StatusCode)
	}
}

func TestParseRetryAfter(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"0", 0},
		{"2", 2 * time.Second},
		{"30", 30 * time.Second},
		{"not a number", 0},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := parseRetryAfter(tc.in); got != tc.want {
				t.Errorf("parseRetryAfter(%q)=%v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestShouldRetryStatus(t *testing.T) {
	retryable := []int{429, 500, 502, 503, 504, 599}
	notRetryable := []int{200, 201, 204, 301, 400, 401, 403, 404, 422}

	for _, c := range retryable {
		if !shouldRetryStatus(c) {
			t.Errorf("shouldRetryStatus(%d)=false, want true", c)
		}
	}
	for _, c := range notRetryable {
		if shouldRetryStatus(c) {
			t.Errorf("shouldRetryStatus(%d)=true, want false", c)
		}
	}
}

func TestContextCancellationStopsRetry(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	d := newDoerForTest(server.Client())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	_, _ = d.Do(req)
	if got := atomic.LoadInt32(&calls); got >= int32(DefaultMaxAttempts) {
		t.Errorf("server calls=%d, want <%d after cancellation", got, DefaultMaxAttempts)
	}
}

func TestHeaderEditorSetsUserAgent(t *testing.T) {
	editor := headerEditor("MyApp/1.0")
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err := editor(context.Background(), req); err != nil {
		t.Fatalf("editor err=%v", err)
	}
	if req.Header.Get("User-Agent") != "MyApp/1.0" {
		t.Errorf("UA=%q, want MyApp/1.0", req.Header.Get("User-Agent"))
	}
	if req.Header.Get("Accept") != "application/json" {
		t.Errorf("Accept=%q, want application/json", req.Header.Get("Accept"))
	}
}

func TestHeaderEditorPreservesExistingAccept(t *testing.T) {
	editor := headerEditor("MyApp/1.0")
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	req.Header.Set("Accept", "application/xml")
	_ = editor(context.Background(), req)
	if req.Header.Get("Accept") != "application/xml" {
		t.Errorf("editor clobbered existing Accept: %q", req.Header.Get("Accept"))
	}
}

func TestAuthEditorAppliesProvider(t *testing.T) {
	provider := auth.BasicAuth{Email: "a@b.cz", APIKey: "secret"}
	editor := authEditor(provider)
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err := editor(context.Background(), req); err != nil {
		t.Fatalf("editor err=%v", err)
	}
	email, key, ok := req.BasicAuth()
	if !ok || email != "a@b.cz" || key != "secret" {
		t.Errorf("BasicAuth not applied: (%q, %q, %v)", email, key, ok)
	}
}

func TestAuthEditorNilProvider(t *testing.T) {
	editor := authEditor(nil)
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	err := editor(context.Background(), req)
	if !errors.Is(err, auth.ErrMissingCredentials) {
		t.Errorf("err=%v, want ErrMissingCredentials", err)
	}
}

// Sanity check the GetBody closure we build in retryingDoer.Do replays
// bytes identically.
func TestBodyReplayRoundTrip(t *testing.T) {
	payload := []byte(`{"hello":"world"}`)
	rc := io.NopCloser(bytes.NewReader(payload))
	got, _ := io.ReadAll(rc)
	if string(got) != string(payload) {
		t.Errorf("readall=%q, want %q", got, payload)
	}
}
