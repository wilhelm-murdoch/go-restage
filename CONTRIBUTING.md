# Contributing

Thanks for your interest in improving go-restage! This is a small, API-agnostic toolkit for building RESTful API clients in Go - a configurable HTTP client, a path/query builder, structured errors, and pluggable auth. It carries no knowledge of any particular API, and contributions that keep it that way are very welcome.

## Getting started

You'll need **Go 1.26+**. Clone the repository and, from its root:

```sh
make test    # unit tests
make race    # unit tests under the race detector (needs CGO)
make lint    # golangci-lint (pinned via go run, no global install needed)
make vet     # go vet
make cover   # unit tests with coverage
make fmt     # gofmt the tree
```

All of these run in CI, so it's worth getting them green locally first. `make lint` downloads and runs the exact pinned linter version on first use - no separate install step.

## How the code is organized

The package has no external dependencies; it is built entirely on the standard library. Each file owns one concern:

- `client.go` - the `Client`, its functional `Option`s, and the JSON, binary, and multipart request methods.
- `path.go` - the `Path` builder (escaped `Seg`, verbatim `Lit`, query helpers).
- `errors.go` - the structured `*Error` type and the `Is*` / `HasStatus` helpers.
- `auth.go` - the `Authenticator` seam and the built-in `BearerToken`.
- `debug.go` - the optional `WithDebug` request/response tracer.

Prefer adding transport-level behavior that any API would want, not behavior specific to one API. Anything domain-specific belongs in the typed client that wraps this package, not here.

## Adding or changing behavior

1. Build paths with the `Path` builder, never by hand-concatenating strings. User-supplied values go through `Seg` (escaped); only trusted, fixed names go through `Lit` / `NewPath`.
2. Keep error messages in the existing `"%s: ...: %w"` shape, prefixed with the client's error prefix.
3. New behavior needs a test. Tests use `net/http/httptest` for real HTTP round-trips - no mocking - following the `newTestClient` helper in `rest_test.go`.
4. Anything touching `BearerToken` or other shared state must stay safe under `-race` (see `TestBearerTokenConcurrent`); run `make race`.

## Pull requests

- Keep changes focused; separate unrelated work into separate PRs.
- Write clear, imperative commit messages explaining the *why*.
- Make sure `make test`, `make race`, `make lint`, and `make vet` pass.
- New behavior needs tests.

By contributing, you agree that your work is licensed under the project's [MIT License](LICENSE).
