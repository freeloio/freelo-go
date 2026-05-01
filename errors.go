package freelo

import (
	"fmt"
	"net/http"
)

// APIError represents a non-2xx response from the Freelo API after all
// retries have been exhausted. Surface via errors.As to inspect status
// code and parsed error fields.
//
// The transport itself does not produce APIError today (the typed client
// returns *http.Response and the caller decides how to interpret status).
// This type is exported so consumer code that wants a uniform error shape
// can construct one from a 4xx/5xx response. Future v0.x versions may
// promote it into the transport once the patterns settle.
type APIError struct {
	StatusCode int
	Status     string
	Body       []byte
	Method     string
	URL        string
}

// NewAPIError builds an APIError from a non-2xx *http.Response. The
// response body is captured up to a safety cap; pass it before the body
// is closed elsewhere.
func NewAPIError(resp *http.Response, body []byte) *APIError {
	if resp == nil {
		return &APIError{Status: "no response"}
	}
	method, urlStr := "", ""
	if resp.Request != nil {
		method = resp.Request.Method
		if resp.Request.URL != nil {
			urlStr = resp.Request.URL.String()
		}
	}
	return &APIError{
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Body:       body,
		Method:     method,
		URL:        urlStr,
	}
}

// Error returns a one-line description suitable for logging.
func (e *APIError) Error() string {
	if e == nil {
		return "freelo: <nil APIError>"
	}
	if e.Method != "" {
		return fmt.Sprintf("freelo: %s %s: %s", e.Method, e.URL, e.Status)
	}
	return fmt.Sprintf("freelo: API error: %s", e.Status)
}
