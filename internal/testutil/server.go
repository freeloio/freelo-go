// Package testutil contains shared httptest helpers used by the SDK's
// own unit tests. Not exported — internal/ keeps it out of the public
// API surface.
package testutil

import (
	"net/http"
	"net/http/httptest"
)

// NewTLSServer wraps httptest.NewTLSServer with the supplied handler.
// The returned server's Client() trusts the test certificate, so SDK
// tests can WithHTTPClient(server.Client()) and reach an https:// URL.
func NewTLSServer(handler http.HandlerFunc) *httptest.Server {
	return httptest.NewTLSServer(handler)
}
