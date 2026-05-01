package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBasicAuth_Apply(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		auth    BasicAuth
		wantErr error
	}{
		{
			name: "ok",
			auth: BasicAuth{Email: "alice@example.com", APIKey: "secret"},
		},
		{
			name:    "empty email",
			auth:    BasicAuth{Email: "", APIKey: "secret"},
			wantErr: ErrMissingCredentials,
		},
		{
			name:    "empty api key",
			auth:    BasicAuth{Email: "alice@example.com", APIKey: ""},
			wantErr: ErrMissingCredentials,
		},
		{
			name:    "both empty",
			auth:    BasicAuth{},
			wantErr: ErrMissingCredentials,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			err := tc.auth.Apply(context.Background(), req)

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err=%v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}

			user, pass, ok := req.BasicAuth()
			if !ok {
				t.Fatal("Authorization header not set as Basic")
			}
			if user != tc.auth.Email || pass != tc.auth.APIKey {
				t.Fatalf("got (%q, %q), want (%q, %q)", user, pass, tc.auth.Email, tc.auth.APIKey)
			}
		})
	}
}

func TestCredentialsFunc_Apply(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		t.Parallel()

		var called int
		fn := CredentialsFunc(func(_ context.Context) (string, string, error) {
			called++
			return "bob@example.com", "key123", nil
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if err := fn.Apply(context.Background(), req); err != nil {
			t.Fatalf("Apply: %v", err)
		}
		if called != 1 {
			t.Fatalf("function called %d times, want 1", called)
		}

		user, pass, ok := req.BasicAuth()
		if !ok || user != "bob@example.com" || pass != "key123" {
			t.Fatalf("got (%q, %q, %v)", user, pass, ok)
		}
	})

	t.Run("function returns error", func(t *testing.T) {
		t.Parallel()

		sentinel := errors.New("vault unreachable")
		fn := CredentialsFunc(func(_ context.Context) (string, string, error) {
			return "", "", sentinel
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		err := fn.Apply(context.Background(), req)
		if !errors.Is(err, sentinel) {
			t.Fatalf("got %v, want %v", err, sentinel)
		}
	})

	t.Run("empty credentials without error", func(t *testing.T) {
		t.Parallel()

		fn := CredentialsFunc(func(_ context.Context) (string, string, error) {
			return "", "", nil
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		err := fn.Apply(context.Background(), req)
		if !errors.Is(err, ErrMissingCredentials) {
			t.Fatalf("err=%v, want ErrMissingCredentials", err)
		}
	})
}

// Compile-time assertion that BasicAuth and CredentialsFunc satisfy
// the Provider interface. If either drifts, this fails to build.
var _ Provider = BasicAuth{}
var _ Provider = CredentialsFunc(nil)
