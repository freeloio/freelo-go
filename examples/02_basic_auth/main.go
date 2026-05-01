// Basic Auth with explicit error handling. The /users/me endpoint is the
// canonical "are my credentials valid?" call.
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

func main() {
	creds := auth.BasicAuth{
		Email:  os.Getenv("FREELO_EMAIL"),
		APIKey: os.Getenv("FREELO_API_KEY"),
	}

	client, err := freelo.New(
		freelo.WithAuth(creds),
		freelo.WithUserAgent("freelo-go-auth-example/0.1"),
	)
	if err != nil {
		// New() rejects empty credentials lazily — the error surfaces on
		// the first call, not at construction. Construction failures here
		// are configuration mistakes (missing UA, bad base URL).
		log.Fatal(err)
	}

	resp, err := client.API.GetUsersMeWithResponse(context.Background())
	if err != nil {
		if errors.Is(err, auth.ErrMissingCredentials) {
			fmt.Fprintln(os.Stderr, "set FREELO_EMAIL and FREELO_API_KEY")
			os.Exit(2)
		}
		log.Fatal(err)
	}

	switch {
	case resp.StatusCode() == 401:
		log.Fatal("authentication failed — check your API key in Freelo settings")
	case resp.JSON200 == nil:
		log.Fatalf("unexpected response: %s", resp.Status())
	}

	fmt.Printf("authenticated as user %v\n", resp.JSON200)
}
