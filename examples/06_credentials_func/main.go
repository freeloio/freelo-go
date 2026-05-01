// CredentialsFunc lets the SDK look up credentials per request. Useful
// when:
//   - env vars override an OS keyring (CLI pattern)
//   - a multi-tenant server picks credentials by tenant from request context
//   - tokens are stored in a vault that needs to be queried per call
//
// Here we demonstrate the env-vars-then-keyring pattern. The "keyring"
// is stubbed inline; in practice you'd plug in zalando/go-keyring or
// any other secret store on your side — the SDK stays agnostic.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	"github.com/freeloio/freelo-go"
	"github.com/freeloio/freelo-go/auth"
)

// stubKeyring stands in for an OS keyring lookup. Replace with your real
// implementation.
func stubKeyring() (string, string, error) {
	return "", "", errors.New("keyring not configured")
}

func main() {
	provider := auth.CredentialsFunc(func(_ context.Context) (string, string, error) {
		// Env vars take priority for CI / ephemeral environments.
		if e, k := os.Getenv("FREELO_EMAIL"), os.Getenv("FREELO_API_KEY"); e != "" && k != "" {
			return e, k, nil
		}
		// Fall back to a real secret store — keyring, vault, AWS Secrets Manager...
		return stubKeyring()
	})

	client, err := freelo.New(
		freelo.WithAuth(provider),
		freelo.WithUserAgent("freelo-go-credfunc-example/0.1"),
	)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.API.GetUsersMeWithResponse(context.Background())
	if err != nil {
		if errors.Is(err, auth.ErrMissingCredentials) {
			log.Fatal("no credentials in env or keyring")
		}
		log.Fatal(err)
	}

	fmt.Printf("status=%s body=%s\n", resp.Status(), resp.Body)
}
