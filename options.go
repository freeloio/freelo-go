package freelo

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/freeloio/freelo-go/auth"
	"github.com/freeloio/freelo-go/freeloapi"
)

// DefaultBaseURL is the production Freelo API root.
const DefaultBaseURL = "https://api.freelo.io/v1"

// Option configures a Client. Apply via New(opts...).
type Option func(*config) error

// RequestEditorFn is the same shape as freeloapi.RequestEditorFn —
// re-exported so consumers can write editors without importing the
// generated package.
type RequestEditorFn = freeloapi.RequestEditorFn

// config aggregates resolved values. Built up by Options inside New.
type config struct {
	baseURL       string
	auth          auth.Provider
	userAgent     string
	httpClient    *http.Client
	rateInterval  time.Duration
	retryAttempts int
	backoffBase   time.Duration
	backoffMax    time.Duration
	editors       []RequestEditorFn
}

// defaultConfig returns the baseline values applied before any Option runs.
func defaultConfig() *config {
	return &config{
		baseURL:       DefaultBaseURL,
		httpClient:    &http.Client{Timeout: DefaultHTTPTimeout},
		rateInterval:  DefaultRateInterval,
		retryAttempts: DefaultMaxAttempts,
		backoffBase:   DefaultBackoffBase,
		backoffMax:    DefaultBackoffMax,
	}
}

// WithAuth sets the credential provider applied to every request. Required;
// New returns an error if WithAuth is omitted.
func WithAuth(p auth.Provider) Option {
	return func(c *config) error {
		if p == nil {
			return errors.New("freelo: WithAuth: provider must not be nil")
		}
		c.auth = p
		return nil
	}
}

// WithBaseURL overrides the default API root. HTTPS is required —
// http:// URLs are rejected to prevent credential leakage. Trailing
// slashes are trimmed.
func WithBaseURL(raw string) Option {
	return func(c *config) error {
		if raw == "" {
			return errors.New("freelo: WithBaseURL: empty URL")
		}
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("freelo: WithBaseURL: %w", err)
		}
		if u.Scheme != "https" {
			return fmt.Errorf("freelo: WithBaseURL: scheme must be https, got %q", u.Scheme)
		}
		c.baseURL = strings.TrimRight(raw, "/")
		return nil
	}
}

// WithUserAgent sets the User-Agent header. Required; Freelo rejects
// requests without one. The recommended form is `<App>/<Version>
// (<contact>)`, e.g. "MyApp/1.0 (ops@example.com)".
func WithUserAgent(ua string) Option {
	return func(c *config) error {
		if strings.TrimSpace(ua) == "" {
			return errors.New("freelo: WithUserAgent: must not be empty")
		}
		c.userAgent = ua
		return nil
	}
}

// WithHTTPClient overrides the default *http.Client (Timeout: 30s).
// Useful for injecting custom transports, proxies, or mocks in tests.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *config) error {
		if hc == nil {
			return errors.New("freelo: WithHTTPClient: client must not be nil")
		}
		c.httpClient = hc
		return nil
	}
}

// WithRateLimit sets the minimum spacing between requests issued by this
// Client. Pass 0 to disable client-side rate limiting (useful when the
// caller manages it themselves, e.g. via a shared semaphore across
// multiple Clients in the same process).
//
// Default is 2.4s, calibrated to Freelo's published 25 req/min server limit.
func WithRateLimit(interval time.Duration) Option {
	return func(c *config) error {
		if interval < 0 {
			return errors.New("freelo: WithRateLimit: interval must not be negative")
		}
		c.rateInterval = interval
		return nil
	}
}

// WithRetry configures retry behavior on transient failures (network
// errors, 429, 5xx). attempts counts the initial try (so attempts=1 means
// no retry). base and max bound the exponential backoff window.
//
// Defaults: attempts=3, base=500ms, max=8s.
func WithRetry(attempts int, base, max time.Duration) Option {
	return func(c *config) error {
		if attempts < 1 {
			return errors.New("freelo: WithRetry: attempts must be >= 1")
		}
		if base <= 0 || max <= 0 || max < base {
			return errors.New("freelo: WithRetry: require 0 < base <= max")
		}
		c.retryAttempts = attempts
		c.backoffBase = base
		c.backoffMax = max
		return nil
	}
}

// WithRequestEditor appends a request editor that runs after the SDK's
// built-in auth + User-Agent editors. Use for adding custom headers
// (correlation IDs, feature flags, ...) or logging.
//
// Editors run in registration order. Returning an error short-circuits
// the request before it leaves the process.
func WithRequestEditor(fn RequestEditorFn) Option {
	return func(c *config) error {
		if fn == nil {
			return errors.New("freelo: WithRequestEditor: editor must not be nil")
		}
		c.editors = append(c.editors, fn)
		return nil
	}
}
