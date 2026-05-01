// Quickstart: build a Freelo client and call one endpoint.
//
//	export FREELO_EMAIL="you@example.com"
//	export FREELO_API_KEY="…"
//	go run ./examples/01_quickstart
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
		freelo.WithUserAgent("freelo-go-quickstart/0.1 (example)"),
	)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.API.GetProjectsWithResponse(context.Background(), nil)
	if err != nil {
		log.Fatal(err)
	}
	if resp.JSON200 == nil {
		fmt.Println("no projects (or unexpected response shape)")
		return
	}
	for _, p := range *resp.JSON200 {
		if p.Id != nil && p.Name != nil {
			fmt.Printf("%d  %s\n", *p.Id, *p.Name)
		}
	}
}
