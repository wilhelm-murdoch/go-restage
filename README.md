# go-restage

[![CI](https://github.com/wilhelm-murdoch/go-restage/actions/workflows/ci.yaml/badge.svg)](https://github.com/wilhelm-murdoch/go-restage/actions/workflows/ci.yaml)
[![Go Reference](https://pkg.go.dev/badge/github.com/wilhelm-murdoch/go-restage.svg)](https://pkg.go.dev/github.com/wilhelm-murdoch/go-restage)
[![Go Report Card](https://goreportcard.com/badge/github.com/wilhelm-murdoch/go-restage)](https://goreportcard.com/report/github.com/wilhelm-murdoch/go-restage)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/wilhelm-murdoch/go-restage/badge)](https://scorecard.dev/viewer/?uri=github.com/wilhelm-murdoch/go-restage)

A small, reusable toolkit for building RESTful API clients in Go: a configurable HTTP client with pluggable authentication, JSON and multipart helpers, a path/query builder, and a structured error type. It carries no knowledge of any particular API.

## Installation

```sh
go get github.com/wilhelm-murdoch/go-restage
```

Note that the package name is `restage`, not `go-restage`:

```go
import "github.com/wilhelm-murdoch/go-restage" // used as restage.New(...), restage.Path, ...
```

## What's in the box

| Piece | Type | Purpose |
| --- | --- | --- |
| Client | `restage.Client` | HTTP transport, JSON verbs, binary + multipart helpers |
| Path | `restage.Path` | Escaped path + query string builder |
| Errors | `restage.Error` + `Is*` helpers | Structured 4xx/5xx errors |
| Auth | `restage.Authenticator` + `restage.BearerToken` | Pluggable request authentication |
| Debug | `restage.WithDebug` | Request/response tracing for development |

## Client

```go
client := restage.New("https://api.example.com",
	restage.WithAuthenticator(restage.NewBearerToken("token")),
	restage.WithUserAgent("my-sdk/1.0"),
	restage.WithErrorPrefix("mysdk"),
)

var out struct {
	Items []string `json:"items"`
}
if err := client.Get(ctx, "/v1/things", &out); err != nil {
	// ...
}
```

### Options

| Option | Effect |
| --- | --- |
| `WithHTTPClient(*http.Client)` | Use a custom HTTP client |
| `WithTimeout(time.Duration)` | Set the request timeout |
| `WithUserAgent(string)` | Override the `User-Agent` header (default `"go-restage"`) |
| `WithAuthenticator(Authenticator)` | Apply auth to every request |
| `WithErrorPrefix(string)` | Prefix on returned error messages (default `"restage"`) |
| `WithInsecureSkipVerify()` | Disable TLS verification |
| `WithDebug(io.Writer)` | Trace every request/response to the writer (see [Debugging](#debugging)) |

Options apply in order; later options win (e.g. `WithHTTPClient` after `WithTimeout` replaces the whole client).

### Requests

- `Get`, `Post`, `Patch`, `Put`, `Delete` - JSON in/out. Pass `nil` for the body to send none, and `nil` for `out` to discard the response.
- `GetBinary(ctx, path) (io.ReadCloser, string, error)` - raw downloads (images, files). The caller closes the reader; the string is the `Content-Type`.
- `PostMultipart(ctx, path, fields, files, out)` - streaming `multipart/form-data` uploads via `io.Pipe`.

```go
files := []restage.MultipartFile{{
	Field:    "file",
	Filename: "cover.jpg",
	Reader:   r, // any io.Reader
}}

err := client.PostMultipart(ctx, "/v1/upload", map[string]string{"title": "x"}, files, nil)
```

A successful response with an empty body is **not** an error: decoding a zero-length body into `out` is a no-op.

## Path

`Path` builds a request path and its query string in one place, escaping user input and skipping empty segments so optional IDs can be passed unconditionally.

```go
restage.NewPath("/api").Seg("lib_1").Lit("items").String()
// "/api/lib_1/items"

restage.NewPath("/api").Seg("a b/c").String()
// "/api/a%20b%2Fc"  (Seg escapes)

restage.NewPath("/api").Lit("items").Seg(id).Flag("hard", true).String()
// "/api/items/<id>?hard=1"
```

| Method | Behavior |
| --- | --- |
| `NewPath(prefix)` | Start from a trusted, verbatim prefix (`/api`, `/login`, `""`) |
| `Seg(...string)` | Append escaped segments; empty values are skipped |
| `Lit(...string)` | Append verbatim segments (trusted sub-resource names) |
| `Query(url.Values)` | Merge query parameters |
| `Set(key, val)` | Add one query parameter |
| `Flag(key, bool)` | Add `key=1` when true, nothing when false |
| `String()` | Render `prefix/seg/...?query` (query keys sorted by `url.Values.Encode`) |

> Never pass user input to `Lit` or `NewPath` - those are verbatim. Anything
> caller-supplied goes through `Seg`.

## Errors

Any 4xx/5xx response becomes a `*restage.Error` (response body truncated to 4 KB). The `Prefix` comes from `WithErrorPrefix`.

```go
err := client.Get(ctx, "/v1/missing", nil)

var apiErr *restage.Error
if errors.As(err, &apiErr) {
	fmt.Println(apiErr.StatusCode, apiErr.Message)
}

if restage.IsNotFound(err) { /* 404 */ }
```

Helpers: `IsNotFound` (404), `IsUnauthorized` (401), `IsForbidden` (403), `IsBadRequest` (400), and the general `HasStatus(err, code)`.

A consumer can re-export the error type so callers never need to import this package:

```go
type Error = restage.Error // errors.As works against either name
```

## Authentication

`Authenticator` is the seam that makes the toolkit reusable across APIs with different auth schemes:

```go
type Authenticator interface {
	Authenticate(req *http.Request)
}
```

`BearerToken` ships in the box and supports updating the token after a login:

```go
auth := restage.NewBearerToken("")
client := restage.New(baseURL, restage.WithAuthenticator(auth))

// after authenticating:
auth.SetToken(resp.Token)
```

For anything else (API-key header, signed requests, OAuth2 refresh), pass a custom type or an inline `AuthenticatorFunc`:

```go
restage.WithAuthenticator(restage.AuthenticatorFunc(func(req *http.Request) {
	req.Header.Set("X-API-Key", key)
}))
```

## Debugging

`WithDebug` traces every request and response - method, URL, headers, and body - to the given writer. It is a development aid, not a production logger.

```go
client := restage.New(baseURL, restage.WithDebug(os.Stderr))
```

The trace restores request and response bodies after reading them, so the real exchange is unaffected. Multipart request bodies are omitted to preserve streaming, non-textual responses (images, downloads) are skipped, and bodies larger than 8 KB are truncated.

> **Security:** `Authorization`, `Cookie`, and similar credential headers are redacted, but **bodies are printed in the clear** and may contain secrets (such as login credentials). Never point `WithDebug` at a server reachable over an untrusted network, and scrub the output before sharing it.

## Wrapping it in a typed client

```go
type Client struct {
	restage *restage.Client
	auth    *restage.BearerToken
}

func NewClient(baseURL string, opts ...Option) *Client {
	c := &Client{auth: restage.NewBearerToken("")}

	// ... collect options into restageOpts ...
	c.restage = restage.New(baseURL, append([]restage.Option{
		restage.WithUserAgent(defaultUA),
		restage.WithAuthenticator(c.auth),
		restage.WithErrorPrefix("mysdk"),
	}, restageOpts...)...)

	return c
}

// Typed methods build a path and delegate to the embedded client:
func (c *Client) Thing(ctx context.Context, id string) (*Thing, error) {
	var t Thing
	err := c.restage.Get(ctx, restage.NewPath("/v1/things").Seg(id).String(), &t)
	return &t, err
}
```

## Development

```sh
make test    # unit tests
make lint    # golangci-lint incl. gosec (pinned via go run)
make cover   # unit tests with coverage; enforces the 80% floor
make vuln    # govulncheck vulnerability scan (pinned via go run)
make fuzz    # native Go fuzzing (FUZZTIME=10s per target by default)
make vet     # go vet
make fmt     # gofmt the tree
make race    # unit tests with --race
```

The Makefile is the single source of truth for the build: both [GitHub Actions](.github/workflows/ci.yaml) and the upstream [Woodpecker pipeline](.woodpecker/workflow.yaml) call these same targets, so a green local run means a green build on both. GitHub Actions additionally runs the tests across Linux, macOS, and Windows on both the minimum supported Go version and the latest stable release, while separate workflows run [govulncheck](.github/workflows/govulncheck.yaml), [CodeQL](.github/workflows/codeql.yaml), and [OpenSSF Scorecard](.github/workflows/scorecard.yaml) on a weekly schedule, and Dependabot keeps dependencies and pinned actions current.

### Releasing

Push a semver tag and the [release workflow](.github/workflows/release.yaml) re-verifies the tagged commit, then publishes a GitHub Release with generated notes:

```sh
git tag v0.2.0 && git push origin v0.2.0
```

## AI Disclosure

The architecture and base structure of this module are my own. I use AI as a tool to assist with time-consuming work - documentatiodn, tests, and bug hunting - and as a sounding board for structural decisions that keep the package easy to adopt. For a solo developer it's a force multiplier for shipping high-quality code efficiently; simply a tool, not a crutch.

## License

[MIT](LICENSE) © Wilhelm Murdoch
