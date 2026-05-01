// Inject a custom *http.Client — corporate proxies, mTLS, custom
// transports, longer/shorter timeouts. The SDK applies its rate-limit +
// retry layer ON TOP of whatever Client you provide.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/freeloio/freelo-go"
	"github.com/freeloio/freelo-go/auth"
)

func main() {
	tr := &http.Transport{
		// Configure a corporate proxy if the env requires one.
		Proxy: http.ProxyFromEnvironment,
		// Tighter dial timeout than the default 30s for a flaky network.
		ResponseHeaderTimeout: 10 * time.Second,
	}
	if proxy := os.Getenv("HTTPS_PROXY"); proxy != "" {
		if u, err := url.Parse(proxy); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}

	customClient := &http.Client{
		Transport: tr,
		Timeout:   45 * time.Second,
	}

	client, err := freelo.New(
		freelo.WithAuth(auth.BasicAuth{
			Email:  os.Getenv("FREELO_EMAIL"),
			APIKey: os.Getenv("FREELO_API_KEY"),
		}),
		freelo.WithUserAgent("freelo-go-custom-http/0.1"),
		freelo.WithHTTPClient(customClient),
	)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.API.GetUsersMeWithResponse(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("status=%s\n", resp.Status())
}
