// Package auth defines pluggable authentication for the Freelo SDK.
//
// The transport applies a Provider to every outgoing request via Apply.
// BasicAuth covers the production path today (email + API key from
// Freelo settings → Profile → API key). CredentialsFunc lets consumers
// resolve credentials lazily per request — useful for multi-tenant
// servers, OS-keyring-backed CLIs, or any case where credentials live
// outside the SDK.
//
// The Refresher interface is reserved for future OAuth providers and is
// a no-op for BasicAuth. When OAuth lands, the transport will inspect
// 401 responses and call Refresh once before retrying.
package auth

import (
	"context"
	"errors"
	"net/http"
)

// ErrMissingCredentials is returned by built-in providers when credentials
// are unset or empty. Wrap or check via errors.Is in consumer code.
var ErrMissingCredentials = errors.New("freelo: missing credentials")

// Provider authenticates an outgoing HTTP request. Implementations apply
// themselves by mutating the request (typically via SetBasicAuth or
// req.Header.Set("Authorization", ...)). Apply receives a context so
// implementations can call out to async credential sources (vaults,
// keyrings, token endpoints) without blocking indefinitely.
type Provider interface {
	Apply(ctx context.Context, req *http.Request) error
}

// Refresher is implemented by providers whose credentials can be refreshed
// (OAuth access tokens, time-limited STS-style creds, ...). The transport
// calls Refresh at most once per response cycle, on 401, before retrying.
//
// BasicAuth does not implement Refresher — its credentials don't expire.
type Refresher interface {
	Refresh(ctx context.Context) error
}

// BasicAuth applies Freelo's HTTP Basic auth: email as username, API key
// as password. Both fields are required; Apply returns ErrMissingCredentials
// if either is empty so the caller fails fast rather than sending an
// unauthenticated request.
//
// For static credentials read once at startup. For credentials that need
// to be looked up per request (env-var override → keyring fallback, etc.),
// use CredentialsFunc instead.
type BasicAuth struct {
	Email  string
	APIKey string
}

// Apply sets the Authorization header on req using Email + APIKey.
func (b BasicAuth) Apply(_ context.Context, req *http.Request) error {
	if b.Email == "" || b.APIKey == "" {
		return ErrMissingCredentials
	}
	req.SetBasicAuth(b.Email, b.APIKey)
	return nil
}

// CredentialsFunc adapts any per-request credential lookup to the
// Provider interface. The function is called on every outgoing request;
// the returned (email, apiKey) pair is applied as Basic auth.
//
// Typical use:
//
//	auth.CredentialsFunc(func(ctx context.Context) (string, string, error) {
//	    if e, k := os.Getenv("FREELO_EMAIL"), os.Getenv("FREELO_API_KEY"); e != "" && k != "" {
//	        return e, k, nil
//	    }
//	    return myKeyring.Lookup(ctx)
//	})
//
// Returning an error short-circuits the request — the transport surfaces
// the error to the caller without contacting the API.
type CredentialsFunc func(ctx context.Context) (email, apiKey string, err error)

// Apply invokes the function and applies the returned credentials as
// Basic auth. Empty (email, apiKey) returned without error counts as
// missing credentials.
func (f CredentialsFunc) Apply(ctx context.Context, req *http.Request) error {
	email, apiKey, err := f(ctx)
	if err != nil {
		return err
	}
	if email == "" || apiKey == "" {
		return ErrMissingCredentials
	}
	req.SetBasicAuth(email, apiKey)
	return nil
}
