# Freelo SDK for Go

Official Go SDK for [Freelo.io](https://app.freelo.io) — typed HTTP client generated from the public OpenAPI spec, with pluggable auth, automatic rate limiting, and retry-with-backoff.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Reference](https://pkg.go.dev/badge/github.com/freeloio/freelo-go.svg)](https://pkg.go.dev/github.com/freeloio/freelo-go)

## Features

- **Auto-generated from OpenAPI** via [`oapi-codegen`](https://github.com/oapi-codegen/oapi-codegen) — always in sync with the Freelo API.
- **Stdlib-only HTTP** — `net/http` under the hood, no external transport dependencies.
- **Drop-in `*WithBody` escape hatch** for endpoints whose live behavior diverges from the spec.
- **Timezone-correct timestamps** — Freelo emits no-zone wall-clock times in `Europe/Prague`; the SDK parses them back to UTC automatically.
- **Production-grade transport** — 25 req/min rate limiting, exponential-backoff retry on 429/5xx, `Retry-After` honored.
- **Pluggable authentication** — `BasicAuth` ships today; `Provider` interface ready for future OAuth.
- **Per-request credential lookup** for multi-tenant servers (`CredentialsFunc`).

## Installation

```bash
go get github.com/freeloio/freelo-go
```

Requires Go 1.26 or newer.

## Quick Start

```go
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
        freelo.WithUserAgent("MyApp/1.0 (contact@example.com)"),
    )
    if err != nil {
        log.Fatal(err)
    }

    resp, err := client.API.GetProjectsWithResponse(context.Background(), nil)
    if err != nil {
        log.Fatal(err)
    }
    if resp.JSON200 != nil {
        for _, p := range *resp.JSON200 {
            if p.Id != nil && p.Name != nil {
                fmt.Println(*p.Id, *p.Name)
            }
        }
    }
}
```

Get your API key from [Freelo Settings → Profile → API key](https://app.freelo.io/profil/nastaveni).

## Authentication

### Basic Auth (in-memory credentials)

Best for daemons, CI jobs, and any scenario where credentials live in env vars or a secret store you control.

```go
freelo.WithAuth(auth.BasicAuth{
    Email:  os.Getenv("FREELO_EMAIL"),
    APIKey: os.Getenv("FREELO_API_KEY"),
})
```

### CredentialsFunc (per-request lookup)

Resolves credentials on every outgoing request. Useful when:

- env vars override an OS keyring (CLI pattern),
- a multi-tenant server picks credentials per tenant,
- tokens come from a secret store that needs per-call lookup.

```go
provider := auth.CredentialsFunc(func(ctx context.Context) (string, string, error) {
    if e, k := os.Getenv("FREELO_EMAIL"), os.Getenv("FREELO_API_KEY"); e != "" && k != "" {
        return e, k, nil
    }
    return myKeyring.Lookup(ctx)
})

client, _ := freelo.New(
    freelo.WithAuth(provider),
    freelo.WithUserAgent("MyApp/1.0"),
)
```

### OAuth 2.1

Not yet shipped. The `auth.Provider` interface (and its companion `auth.Refresher`) are designed to accept an OAuth provider once Freelo provisions a dedicated `client_id` for third-party SDKs. Track progress in this repo's milestones.

## Configuration

All knobs are functional options on `freelo.New`:

| Option | Default | Notes |
|---|---|---|
| `WithAuth(p)` | — | **Required.** `auth.Provider` impl. |
| `WithUserAgent(ua)` | — | **Required.** Freelo rejects requests without one. |
| `WithBaseURL(url)` | `https://api.freelo.io/v1` | Must be HTTPS. Trailing slash trimmed. |
| `WithHTTPClient(hc)` | `&http.Client{Timeout: 30s}` | Inject custom transport, proxy, mTLS, etc. |
| `WithRateLimit(d)` | `2.4s` | 25 req/min server limit. `0` disables (manage externally). |
| `WithRetry(attempts, base, max)` | `3, 500ms, 8s` | Full-jitter exponential backoff. |
| `WithRequestEditor(fn)` | — | Append custom headers / logging. Runs after auth + UA. |

## Typed responses & raw access

Two ways to call the API:

**Typed (default).** `client.API` is the full `*freeloapi.ClientWithResponses`. Every endpoint has a typed `*WithResponse` method that decodes 2xx/4xx/5xx bodies into struct fields:

```go
resp, err := client.API.GetUsersMeWithResponse(ctx)
// resp.JSON200, resp.JSON401, resp.Status() …
```

**Raw.** `client.Raw` sends an arbitrary request through the same auth + UA + rate-limit + retry pipeline and hands you the bare `*http.Response`:

```go
resp, err := client.Raw(ctx, http.MethodGet, "/users/me", nil)
defer resp.Body.Close()
```

Use `Raw` when an endpoint isn't covered by the spec yet, or when you'd rather decode the body yourself.

## Timestamps

Freelo returns timestamps without timezone (`"2026-04-24T11:12:38"`). The SDK ships a `freelotime.Time` type that:

- accepts both RFC3339-with-zone and the no-zone wall-clock form,
- interprets the no-zone form as `Europe/Prague`,
- normalizes everything to UTC on the way in and out.

The generated client uses `freelotime.Time` for every `format: date-time` field, so typed `*WithResponse` methods Just Work.

## Examples

See [`examples/`](examples) for runnable scenarios:

| | |
|---|---|
| `01_quickstart` | Smallest possible main.go |
| `02_basic_auth` | Explicit error handling on `/users/me` |
| `03_list_projects` | Pagination |
| `04_create_task` | POST with typed body |
| `05_comment_with_files` | File upload + comment with attachment UUID |
| `06_credentials_func` | env-then-keyring lookup pattern |
| `07_custom_http_client` | Inject a custom `*http.Client` |
| `08_raw_passthrough` | `client.Raw` escape hatch |

Run any of them after exporting `FREELO_EMAIL` / `FREELO_API_KEY`:

```bash
go run ./examples/01_quickstart
```

## Rate limiting

Freelo's published server limit is **25 req/min**. The SDK paces outgoing requests with a `2.4s` minimum interval per `*Client`. **A `*Client` is a singleton per process** — creating multiple clients in the same process means multiple rate limiters racing each other against the same server limit.

If you need shared limiting across many clients, set `WithRateLimit(0)` on each and gate them externally (`golang.org/x/sync/semaphore`, etc.).

## Versioning & releases

Independent semver. Tagged via [release-please](https://github.com/googleapis/release-please). Conventional-commit subjects (`feat:`, `fix:`, `chore:`) drive the changelog.

Distribution is the standard Go module proxy — once a tag is pushed, `go get github.com/freeloio/freelo-go@vX.Y.Z` works within minutes (no separate registry, no goreleaser).

## Development

```bash
make help          # list targets
make test          # unit tests with -race
make lint          # gofmt + go vet
make examples      # build every example (catches API drift)
make gen           # download spec, patch spec, regen client, patch time.Time
make gen-check     # CI guard — fails if generated code drifts from spec
```

Spec lives at `spec/freelo-api.yaml` (vendored). The vendored copy is byte-identical to upstream apart from two mechanical edits — a `Client → BusinessClient` rename (collision with `oapi-codegen`'s HTTP `Client` type) and the flattening of scalar `oneOf` parameter schemas to `type: string` (see `scripts/patchspec`; `oapi-codegen` generates non-compiling union types for those, and path/query params are strings on the wire anyway) — plus the post-generation `time.Time → freelotime.Time` patch (see `scripts/patchgen`). A weekly cron in `.github/workflows/update-api-spec.yml` PRs any drift.

## Contributing

Bug reports and PRs welcome. For new features, please open an issue first to discuss the approach — this SDK aims to stay small and predictable, and additions need to fit that goal.

## License

MIT — see [LICENSE](LICENSE).

## Sibling SDKs

- [`@freeloapp/js-sdk`](https://github.com/freeloio/js-sdk) — TypeScript / JavaScript
- [`freeloapp/php-sdk`](https://github.com/freeloio/php-sdk) — PHP
