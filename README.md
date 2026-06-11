# go-restage

A small, reusable toolkit for building RESTful API clients in Go: a configurable HTTP client with pluggable authentication, JSON and multipart helpers, a path/query builder, and a structured error type. It carries no knowledge of any particular API.

## What's in the box

| Piece | Type | Purpose |
| --- | --- | --- |
| Client | `rest.Client` | HTTP transport, JSON verbs, binary + multipart helpers |
| Path | `rest.Path` | Escaped path + query string builder |
| Errors | `rest.Error` + `Is*` helpers | Structured 4xx/5xx errors |
| Auth | `rest.Authenticator` + `rest.BearerToken` | Pluggable request authentication |
| Debug | `rest.WithDebug` | Request/response tracing for development |

## Client

```go
client := rest.New("https://api.example.com",
	rest.WithAuthenticator(rest.NewBearerToken("token")),
	rest.WithUserAgent("my-sdk/1.0"),
	rest.WithErrorPrefix("mysdk"),
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
| `WithErrorPrefix(string)` | Prefix on returned error messages (default `"rest"`) |
| `WithInsecureSkipVerify()` | Disable TLS verification |
| `WithDebug(io.Writer)` | Trace every request/response to the writer (see [Debugging](#debugging)) |

Options apply in order; later options win (e.g. `WithHTTPClient` after `WithTimeout` replaces the whole client).

### Requests

- `Get`, `Post`, `Patch`, `Put`, `Delete` - JSON in/out. Pass `nil` for the body to send none, and `nil` for `out` to discard the response.
- `GetBinary(ctx, path) (io.ReadCloser, string, error)` - raw downloads (images, files). The caller closes the reader; the string is the `Content-Type`.
- `PostMultipart(ctx, path, fields, files, out)` - streaming `multipart/form-data` uploads via `io.Pipe`.

```go
files := []rest.MultipartFile{{
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
rest.NewPath("/api").Seg("lib_1").Lit("items").String()
// "/api/lib_1/items"

rest.NewPath("/api").Seg("a b/c").String()
// "/api/a%20b%2Fc"  (Seg escapes)

rest.NewPath("/api").Lit("items").Seg(id).Flag("hard", true).String()
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

Any 4xx/5xx response becomes a `*rest.Error` (response body truncated to 4 KB). The `Prefix` comes from `WithErrorPrefix`.

```go
err := client.Get(ctx, "/v1/missing", nil)

var apiErr *rest.Error
if errors.As(err, &apiErr) {
	fmt.Println(apiErr.StatusCode, apiErr.Message)
}

if rest.IsNotFound(err) { /* 404 */ }
```

Helpers: `IsNotFound` (404), `IsUnauthorized` (401), `IsForbidden` (403), `IsBadRequest` (400), and the general `HasStatus(err, code)`.

A consumer can re-export the error type so callers never need to import this package:

```go
type Error = rest.Error // errors.As works against either name
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
auth := rest.NewBearerToken("")
client := rest.New(baseURL, rest.WithAuthenticator(auth))

// after authenticating:
auth.SetToken(resp.Token)
```

For anything else (API-key header, signed requests, OAuth2 refresh), pass a custom type or an inline `AuthenticatorFunc`:

```go
rest.WithAuthenticator(rest.AuthenticatorFunc(func(req *http.Request) {
	req.Header.Set("X-API-Key", key)
}))
```

## Debugging

`WithDebug` traces every request and response - method, URL, headers, and body - to the given writer. It is a development aid, not a production logger.

```go
client := rest.New(baseURL, rest.WithDebug(os.Stderr))
```

The trace restores request and response bodies after reading them, so the real exchange is unaffected. Multipart request bodies are omitted to preserve streaming, non-textual responses (images, downloads) are skipped, and bodies larger than 8 KB are truncated.

> **Security:** `Authorization`, `Cookie`, and similar credential headers are redacted, but **bodies are printed in the clear** and may contain secrets (such as login credentials). Never point `WithDebug` at a server reachable over an untrusted network, and scrub the output before sharing it.

## Wrapping it in a typed client

```go
type Client struct {
	rest *rest.Client
	auth *rest.BearerToken
}

func NewClient(baseURL string, opts ...Option) *Client {
	c := &Client{auth: rest.NewBearerToken("")}

	// ... collect options into restOpts ...
	c.rest = rest.New(baseURL, append([]rest.Option{
		rest.WithUserAgent(defaultUA),
		rest.WithAuthenticator(c.auth),
		rest.WithErrorPrefix("mysdk"),
	}, restOpts...)...)

	return c
}

// Typed methods build a path and delegate to the embedded client:
func (c *Client) Thing(ctx context.Context, id string) (*Thing, error) {
	var t Thing
	err := c.rest.Get(ctx, rest.NewPath("/v1/things").Seg(id).String(), &t)
	return &t, err
}
```

## Development

```sh
make test    # unit tests
make lint    # golangci-lint (pinned via go run)
make cover   # unit tests with coverage
make vet     # go vet
make fmt     # gofmt the tree
make race    # unit tests with --race
```

CI (GitHub Actions and Woodpecker) runs the full suite of tests and checks.

## AI Disclosure

The architecture and base structure of this module are my own. I use AI as a tool to assist with time-consuming work - documentatiodn, tests, and bug hunting - and as a sounding board for structural decisions that keep the package easy to adopt. For a solo developer it's a force multiplier for shipping high-quality code efficiently; simply a tool, not a crutch.

## License

[MIT](LICENSE) © Wilhelm Murdoch
