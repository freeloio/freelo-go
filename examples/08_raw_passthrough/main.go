// Use Client.Raw to call endpoints not covered by the typed client (or
// when you'd rather decode the response yourself).
//
// Equivalent to PHP SDK's $freelo->call() and JS SDK's call() helper.
// Goes through the same auth + UA + rate-limit + retry pipeline.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/freeloio/freelo-go"
	"github.com/freeloio/freelo-go/auth"
)

func main() {
	client, err := freelo.New(
		freelo.WithAuth(auth.BasicAuth{
			Email:  os.Getenv("FREELO_EMAIL"),
			APIKey: os.Getenv("FREELO_API_KEY"),
		}),
		freelo.WithUserAgent("freelo-go-raw-example/0.1"),
	)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.Raw(context.Background(), http.MethodGet, "/users/me", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("status=%s\n%s\n", resp.Status, body)
}
