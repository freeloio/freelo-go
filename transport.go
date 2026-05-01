package freelo

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"sync"
	"time"

	"github.com/freeloio/freelo-go/auth"
	"github.com/freeloio/freelo-go/freeloapi"
)

// Default values applied when corresponding Option is not supplied.
const (
	// Freelo publishes a 25 req/min rate limit. Spacing requests by ~2.4s
	// keeps us safely under it; tunable via WithRateLimit (0 disables).
	DefaultRateInterval = 60 * time.Second / 25

	// DefaultMaxAttempts caps total tries per request (initial + retries).
	DefaultMaxAttempts = 3

	// DefaultBackoffBase / DefaultBackoffMax bound exponential backoff.
	DefaultBackoffBase = 500 * time.Millisecond
	DefaultBackoffMax  = 8 * time.Second

	// DefaultHTTPTimeout applies when WithHTTPClient is not used.
	DefaultHTTPTimeout = 30 * time.Second

	// maxResponseBody caps response bodies we read on retry-time drains
	// (the typed client reads its own bodies separately). Guards against
	// pathological upstreams.
	maxResponseBody = 10 * 1024 * 1024
)

// retryingDoer is an oapi-codegen HttpRequestDoer that enforces a
// client-side rate limit, retries transient failures with exponential
// backoff + full jitter, and honors Retry-After headers on 429.
type retryingDoer struct {
	inner       *http.Client
	interval    time.Duration
	maxAttempts int
	backoffBase time.Duration
	backoffMax  time.Duration

	mu          sync.Mutex
	lastRequest time.Time
}

func newRetryingDoer(cfg *config) *retryingDoer {
	return &retryingDoer{
		inner:       cfg.httpClient,
		interval:    cfg.rateInterval,
		maxAttempts: cfg.retryAttempts,
		backoffBase: cfg.backoffBase,
		backoffMax:  cfg.backoffMax,
	}
}

// Do is the HttpRequestDoer contract from oapi-codegen.
func (d *retryingDoer) Do(req *http.Request) (*http.Response, error) {
	// Capture the request body so retries can replay the same payload.
	// oapi-codegen hands us bytes.Reader-backed bodies that already have
	// GetBody set, but defensive code here covers caller-supplied readers.
	if req.Body != nil && req.GetBody == nil {
		body, err := io.ReadAll(io.LimitReader(req.Body, maxResponseBody))
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
		_ = req.Body.Close()
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
		req.Body, _ = req.GetBody()
	}

	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt < d.maxAttempts; attempt++ {
		if attempt > 0 && req.GetBody != nil {
			rc, err := req.GetBody()
			if err != nil {
				return nil, fmt.Errorf("rewind body: %w", err)
			}
			req.Body = rc
		}

		d.waitForRateLimit()

		resp, err := d.inner.Do(req)
		lastResp, lastErr = resp, err

		// Network failure → retry with backoff if it looks transient.
		if err != nil {
			if !isRetryableNetErr(err) {
				return nil, err
			}
			sleepWithBackoff(req.Context(), attempt, d.backoffBase, d.backoffMax)
			continue
		}

		// HTTP-level retryable status (429 or 5xx).
		if shouldRetryStatus(resp.StatusCode) && attempt < d.maxAttempts-1 {
			delay := parseRetryAfter(resp.Header.Get("Retry-After"))
			_ = drainAndClose(resp.Body)
			if delay > 0 {
				sleepCtx(req.Context(), delay)
			} else {
				sleepWithBackoff(req.Context(), attempt, d.backoffBase, d.backoffMax)
			}
			continue
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all %d attempts failed: %w", d.maxAttempts, lastErr)
	}
	return lastResp, nil
}

// waitForRateLimit blocks until enough time has elapsed since the last
// request. Disabled when interval == 0.
func (d *retryingDoer) waitForRateLimit() {
	if d.interval == 0 {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	elapsed := time.Since(d.lastRequest)
	if elapsed < d.interval {
		time.Sleep(d.interval - elapsed)
	}
	d.lastRequest = time.Now()
}

func shouldRetryStatus(code int) bool {
	return code == http.StatusTooManyRequests || (code >= 500 && code <= 599)
}

// isRetryableNetErr discriminates transient IO failures from fatal ones
// (context cancelled / deadline exceeded). Today we treat any non-context
// error as transient — the http.Client returns IO-level errors here, not
// semantic ones, since URL parsing already passed.
func isRetryableNetErr(err error) bool {
	return err != nil && err != context.Canceled && err != context.DeadlineExceeded
}

// parseRetryAfter understands both delta-seconds (most common) and HTTP-date
// forms. Returns 0 if unparseable or zero/negative.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := time.ParseDuration(v + "s"); err == nil && secs > 0 {
		return secs
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}

// sleepWithBackoff sleeps for base * 2^attempt + jitter, capped at max.
// "Full jitter" per AWS architecture blog: pick a uniform random value in
// [0, base * 2^attempt].
func sleepWithBackoff(ctx context.Context, attempt int, base, max time.Duration) {
	exp := base << attempt
	if exp > max {
		exp = max
	}
	if exp <= 0 {
		return
	}
	delay := time.Duration(rand.Int64N(int64(exp)))
	sleepCtx(ctx, delay)
}

func sleepCtx(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	if ctx == nil {
		time.Sleep(d)
		return
	}
	select {
	case <-time.After(d):
	case <-ctx.Done():
	}
}

func drainAndClose(body io.ReadCloser) error {
	if body == nil {
		return nil
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(body, maxResponseBody))
	return body.Close()
}

// authEditor produces a freeloapi.RequestEditorFn that runs the supplied
// auth.Provider on each request. Resolved per-request so credentials that
// change at runtime (env vars updated, keyring rotated) take effect on
// the next call without rebuilding the client.
func authEditor(p auth.Provider) freeloapi.RequestEditorFn {
	return func(ctx context.Context, req *http.Request) error {
		if p == nil {
			return auth.ErrMissingCredentials
		}
		return p.Apply(ctx, req)
	}
}

// headerEditor sets the headers Freelo expects on every request. Freelo
// rejects requests without a User-Agent.
func headerEditor(userAgent string) freeloapi.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		req.Header.Set("User-Agent", userAgent)
		if req.Header.Get("Accept") == "" {
			req.Header.Set("Accept", "application/json")
		}
		return nil
	}
}
