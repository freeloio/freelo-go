// List all projects accessible to the authenticated user. Prints id,
// name, and (if known) the project state from the embedded tasklists
// or business client.
//
// Note: GET /projects returns a flat list — there's no pagination cursor
// on this endpoint. For larger workspaces use GET /all-projects (separate
// helper) or filter via params.
package main

import (
	"context"
	"fmt"
	"log"
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
		freelo.WithUserAgent("freelo-go-list-projects/0.1"),
	)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.API.GetProjectsWithResponse(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}
	if resp.JSON200 == nil {
		log.Fatalf("unexpected response: %s", resp.Status())
	}

	for _, p := range *resp.JSON200 {
		id, name := "?", "?"
		if p.Id != nil {
			id = fmt.Sprintf("%d", *p.Id)
		}
		if p.Name != nil {
			name = *p.Name
		}
		fmt.Printf("%-10s %s\n", id, name)
	}
}
