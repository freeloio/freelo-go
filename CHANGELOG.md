# Changelog

All notable changes to the Freelo Go SDK are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.3.0](https://github.com/freeloio/freelo-go/compare/v0.2.1...v0.3.0) (2026-08-03)


### ⚠ BREAKING CHANGES

* NoteIdParam is an alias for string, so GetNote, EditNote and DeleteNote (with their WithResponse and NewXRequest variants) now take `noteId string` instead of `int`. Pass the numeric id as a string ("12345") or the note uuid.

### Features

* accept note id or uuid, flatten oneOf parameter schemas ([#13](https://github.com/freeloio/freelo-go/issues/13)) ([5addba4](https://github.com/freeloio/freelo-go/commit/5addba4ef1bd8ff5425787e7c3115a1f567e6938))

## [0.2.1](https://github.com/freeloio/freelo-go/compare/v0.2.0...v0.2.1) (2026-07-10)


### Features

* add getTaskLabelColors (list task-label colors) endpoint ([022aa9a](https://github.com/freeloio/freelo-go/commit/022aa9a73ff1590bda94c70b011098909471aa7b))
* add getTaskLabelColors (list task-label colors) endpoint ([d7391bc](https://github.com/freeloio/freelo-go/commit/d7391bc3d62b94ce7329c970112c9a3947dd0607))

## [Unreleased]

## [0.2.0](https://github.com/freeloio/freelo-go/compare/v0.1.0...v0.2.0) (2026-06-03)

### ⚠ BREAKING CHANGES

- `FindAvailableProjectLabels` response field renamed
  `Label` → `Labels` (spec fix — the live API returns `labels`).
- Project workers in `GetProjectWorkers` are now typed
  `UserWithEmail` (adds `Email`) instead of `UserBasic`.

### Features

- Regenerated client from the latest public spec
  ([1f9dc73](https://github.com/freeloio/freelo-go/commit/1f9dc73779e3031a1b06369e1fb4b045c6b8c74c)):
  - `EditTasklistWithResponse` — edit a tasklist (`POST /tasklist/{id}/edit`).
  - Checklist-item (taskcheck) operations: `EditTaskcheckWithResponse`,
    `DeleteTaskcheckWithResponse`, `FinishTaskcheckWithResponse`,
    `ActivateTaskcheckWithResponse`.
  - `FindAvailableTaskLabelsWithResponse` — list task labels usable by
    the caller.
  - `GetCurrentUser` response now includes `Email`, `Fullname` and
    `MentionKey` (for building `@mention` spans in comments).
  - Optional `Priority` field on `CreateTasklist` body — positional
    ordering of the new tasklist within the project.
  - Collection endpoints accept `page` as an alias of the `p`
    pagination parameter.

### Build System

- Switch release-please to manifest mode, stay in 0.x
  ([fce95fb](https://github.com/freeloio/freelo-go/commit/fce95fbc683f40d7fc6e8ff2c0a6ef21939c260c))

## [0.1.0] — 2026-05-01

Initial release. The SDK was extracted from
[`freelo-cli`](https://github.com/freeloio/freelo-cli) so other Go
projects can consume the Freelo API client without pulling in CLI
plumbing.

### Added

- **Typed OpenAPI client** generated from the public Freelo spec via
  [`oapi-codegen`](https://github.com/oapi-codegen/oapi-codegen),
  exposed as `client.API` (`*freeloapi.ClientWithResponses`). Every
  endpoint has a `*WithResponse` method with decoded 2xx/4xx/5xx
  bodies.
- **`client.Raw`** escape hatch — sends an arbitrary request through
  the same auth + UA + rate-limit + retry pipeline and returns the
  raw `*http.Response`. Use it for endpoints not yet covered by the
  spec or when you'd rather decode bodies yourself.
- **Production-grade transport** built on stdlib `net/http`:
  - 25 req/min rate limiting (configurable via `WithRateLimit`,
    `0` disables for externally managed limiting).
  - Exponential-backoff retry on 429 / 5xx with full jitter
    (configurable via `WithRetry`); `Retry-After` honored.
- **Pluggable authentication** via the `auth.Provider` interface:
  - `auth.BasicAuth` — in-memory email + API key.
  - `auth.CredentialsFunc` — per-request credential lookup, useful
    for env-vs-keyring CLI patterns and multi-tenant servers.
  - `auth.Refresher` companion interface ready for future OAuth.
- **`freelotime.Time`** — a timezone-correct timestamp type that
  parses Freelo's no-zone wire format
  (`"2026-04-24T11:12:38"`) as `Europe/Prague` and normalizes to UTC.
  The generated client uses it for every `format: date-time` field,
  so typed `*WithResponse` decoders work out of the box.
- **Functional options** on `freelo.New`: `WithAuth`,
  `WithUserAgent` (both required), `WithBaseURL`, `WithHTTPClient`,
  `WithRateLimit`, `WithRetry`, `WithRequestEditor`.
- **Eight runnable examples** in `examples/` covering quickstart,
  explicit error handling, pagination, typed POST bodies, file
  upload + comment, env-then-keyring credential lookup, custom
  `*http.Client` injection, and the `Raw` passthrough.
- **Make targets** for development: `test` (unit tests with `-race`),
  `lint` (`gofmt` + `go vet`), `examples` (build every example to
  catch API drift), `gen` (download spec, regenerate client, patch
  `time.Time → freelotime.Time`), `gen-check` (CI guard).

[Unreleased]: https://github.com/freeloio/freelo-go/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/freeloio/freelo-go/releases/tag/v0.1.0
