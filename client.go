package freelo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/freeloio/freelo-go/freeloapi"
)

// Client is the entry point to the Freelo SDK. Build one with New, then
// reach for typed responses via Client.API or send arbitrary requests via
// Client.Raw. A Client is safe for concurrent use.
//
// Internally a Client wraps a *freeloapi.ClientWithResponses with the
// rate-limit + retry transport, the user-supplied auth provider, and any
// extra request editors. Tear-down is the http.Client's responsibility —
// the SDK does not own goroutines.
type Client struct {
	// API is the typed, fully-generated Freelo client. All endpoints from
	// spec/freelo-api.yaml are reachable through it (e.g.
	// API.GetProjectsWithResponse, API.CreateCommentWithBody).
	API *freeloapi.ClientWithResponses

	// rawClient is the underlying *freeloapi.Client used by Raw to send
	// ad-hoc requests through the same transport pipeline.
	rawClient *freeloapi.Client

	cfg *config
}

// New builds a Client from Options. WithAuth and WithUserAgent are
// required — New returns an error if either is omitted. Other options
// fall back to documented defaults (see DefaultBaseURL, DefaultRateInterval,
// DefaultMaxAttempts, DefaultHTTPTimeout).
func New(opts ...Option) (*Client, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(cfg); err != nil {
			return nil, err
		}
	}
	if cfg.auth == nil {
		return nil, fmt.Errorf("freelo: New: WithAuth is required")
	}
	if cfg.userAgent == "" {
		return nil, fmt.Errorf("freelo: New: WithUserAgent is required")
	}

	doer := newRetryingDoer(cfg)

	// Order matters: auth → UA → user editors. User editors run last so
	// they can see (or even override) headers set by the SDK.
	editors := make([]freeloapi.RequestEditorFn, 0, 2+len(cfg.editors))
	editors = append(editors, authEditor(cfg.auth))
	editors = append(editors, headerEditor(cfg.userAgent))
	editors = append(editors, cfg.editors...)

	clientOpts := []freeloapi.ClientOption{
		freeloapi.WithHTTPClient(doer),
	}
	for _, ed := range editors {
		clientOpts = append(clientOpts, freeloapi.WithRequestEditorFn(ed))
	}

	api, err := freeloapi.NewClientWithResponses(cfg.baseURL, clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("freelo: New: build typed client: %w", err)
	}

	// Pull out the underlying *freeloapi.Client so Raw can reuse the same
	// transport + editor stack without rebuilding it. NewClientWithResponses
	// always wraps a concrete *Client, so the assertion is sound.
	inner, ok := api.ClientInterface.(*freeloapi.Client)
	if !ok {
		return nil, fmt.Errorf("freelo: New: unexpected ClientInterface impl %T", api.ClientInterface)
	}

	return &Client{API: api, rawClient: inner, cfg: cfg}, nil
}

// Raw sends an ad-hoc HTTP request through the SDK's transport pipeline
// (auth + User-Agent + rate limit + retry) for endpoints not covered by
// the generated typed client, or when callers need the bare *http.Response.
//
// Path may be relative (joined to the configured base URL) or absolute
// (passed through unchanged). Body may be nil for GET/DELETE.
//
// The returned *http.Response is the caller's responsibility to close.
func (c *Client) Raw(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	if c == nil || c.rawClient == nil {
		return nil, fmt.Errorf("freelo: Raw: client is nil")
	}

	target := path
	if !strings.HasPrefix(path, "http://") && !strings.HasPrefix(path, "https://") {
		base := strings.TrimRight(c.cfg.baseURL, "/")
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		target = base + path
	}

	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, fmt.Errorf("freelo: Raw: build request: %w", err)
	}

	for _, ed := range c.rawClient.RequestEditors {
		if err := ed(ctx, req); err != nil {
			return nil, fmt.Errorf("freelo: Raw: editor failed: %w", err)
		}
	}

	return c.rawClient.Client.Do(req)
}
