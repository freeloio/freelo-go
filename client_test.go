package freelo

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freeloio/freelo-go/auth"
)

func TestNew_RequiresAuth(t *testing.T) {
	t.Parallel()
	_, err := New(WithUserAgent("MyApp/1.0"))
	if err == nil {
		t.Error("New without WithAuth returned nil error")
	}
}

func TestNew_RequiresUserAgent(t *testing.T) {
	t.Parallel()
	_, err := New(WithAuth(auth.BasicAuth{Email: "a@b.cz", APIKey: "k"}))
	if err == nil {
		t.Error("New without WithUserAgent returned nil error")
	}
}

func TestNew_DefaultsApply(t *testing.T) {
	t.Parallel()
	c, err := New(
		WithAuth(auth.BasicAuth{Email: "a@b.cz", APIKey: "k"}),
		WithUserAgent("MyApp/1.0"),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.cfg.baseURL != DefaultBaseURL {
		t.Errorf("baseURL=%q, want %q", c.cfg.baseURL, DefaultBaseURL)
	}
	if c.API == nil {
		t.Error("Client.API is nil")
	}
	if c.rawClient == nil {
		t.Error("Client.rawClient is nil")
	}
}

func TestRaw_AppliesAuthAndUserAgent(t *testing.T) {
	t.Parallel()

	var seenUA, seenAuth, seenPath string
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenUA = r.Header.Get("User-Agent")
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer tlsServer.Close()

	c, err := New(
		WithBaseURL(tlsServer.URL),
		WithAuth(auth.BasicAuth{Email: "a@b.cz", APIKey: "secret"}),
		WithUserAgent("MyApp/1.0"),
		WithHTTPClient(tlsServer.Client()),
		WithRateLimit(0),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := c.Raw(context.Background(), http.MethodGet, "/projects", nil)
	if err != nil {
		t.Fatalf("Raw: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != `{"ok":true}` {
		t.Errorf("body=%q, want {\"ok\":true}", body)
	}

	if seenUA != "MyApp/1.0" {
		t.Errorf("server saw UA=%q, want MyApp/1.0", seenUA)
	}
	if !strings.HasPrefix(seenAuth, "Basic ") {
		t.Errorf("server saw Authorization=%q, want Basic …", seenAuth)
	}
	if seenPath != "/projects" {
		t.Errorf("server saw path=%q, want /projects", seenPath)
	}
}

func TestRaw_AbsoluteURLBypassesBaseURL(t *testing.T) {
	t.Parallel()

	var seenHost string
	tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer tlsServer.Close()

	c, err := New(
		WithBaseURL("https://api.freelo.io/v1"),
		WithAuth(auth.BasicAuth{Email: "a@b.cz", APIKey: "secret"}),
		WithUserAgent("MyApp/1.0"),
		WithHTTPClient(tlsServer.Client()),
		WithRateLimit(0),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resp, err := c.Raw(context.Background(), http.MethodGet, tlsServer.URL+"/foo", nil)
	if err != nil {
		t.Fatalf("Raw: %v", err)
	}
	defer resp.Body.Close()

	if !strings.Contains(seenHost, "127.0.0.1") {
		t.Errorf("server saw host=%q, expected absolute URL routed to test server", seenHost)
	}
}
