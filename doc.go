// Package freelo is the Go SDK for the Freelo.io project-management API.
//
// It provides a typed HTTP client (generated from the public OpenAPI
// spec), pluggable authentication, automatic rate limiting, and retry
// with exponential backoff on transient failures.
//
// # Quick start
//
//	client, err := freelo.New(
//	    freelo.WithAuth(auth.BasicAuth{Email: email, APIKey: apiKey}),
//	    freelo.WithUserAgent("MyApp/1.0 (contact@example.com)"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	resp, err := client.API.GetProjectsWithResponse(ctx, nil)
//	for _, p := range *resp.JSON200 {
//	    fmt.Println(*p.Id, *p.Name)
//	}
//
// See the examples/ directory for runnable scenarios.
package freelo
