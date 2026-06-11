// Package rest is a small, reusable toolkit for building RESTful API
// clients: a configurable HTTP client with pluggable authentication,
// JSON and multipart helpers, a path/query builder, and a structured
// error type. It carries no knowledge of any particular API and is
// intended to be wrapped by a typed, domain-specific client.
package rest

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// DefaultUserAgent is used when no WithUserAgent option is given.
const DefaultUserAgent = "go-restage"

// Client is a configurable REST HTTP client. It is safe for concurrent
// use once configured.
type Client struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
	auth       Authenticator
	errPrefix  string
	debug      io.Writer
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets a custom *http.Client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithTimeout sets the request timeout on the underlying *http.Client.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) { c.httpClient.Timeout = d }
}

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// WithAuthenticator sets the Authenticator applied to every request.
func WithAuthenticator(a Authenticator) Option {
	return func(c *Client) { c.auth = a }
}

// WithErrorPrefix sets the prefix used on returned errors (e.g. the
// wrapping package name). It defaults to "rest".
func WithErrorPrefix(prefix string) Option {
	return func(c *Client) { c.errPrefix = prefix }
}

// WithInsecureSkipVerify disables TLS certificate verification.
func WithInsecureSkipVerify() Option {
	return func(c *Client) {
		transport, ok := c.httpClient.Transport.(*http.Transport)
		if !ok || transport == nil {
			transport = http.DefaultTransport.(*http.Transport).Clone()
		}

		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}

		transport.TLSClientConfig.InsecureSkipVerify = true
		c.httpClient.Transport = transport
	}
}

// New returns a Client for the API at baseURL.
func New(baseURL string, opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: time.Minute},
		baseURL:    strings.TrimRight(baseURL, "/"),
		userAgent:  DefaultUserAgent,
		errPrefix:  "rest",
	}

	for _, opt := range opts {
		opt(c)
	}

	// Apply the debug wrapper last so it captures whatever transport the
	// other options settled on (custom client, skipped TLS verification).
	if c.debug != nil {
		base := c.httpClient.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		c.httpClient.Transport = newDebugTransport(base, c.debug)
	}

	return c
}

// BaseURL returns the configured base URL without a trailing slash.
func (c *Client) BaseURL() string { return c.baseURL }

// Get performs a GET request against path and decodes the JSON response
// into out. Pass nil for out to discard the response body.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}

// Post performs a POST request with an optional JSON body.
func (c *Client) Post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}

// Patch performs a PATCH request with an optional JSON body.
func (c *Client) Patch(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPatch, path, body, out)
}

// Put performs a PUT request with an optional JSON body.
func (c *Client) Put(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPut, path, body, out)
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodDelete, path, nil, out)
}

func (c *Client) newRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("%s: building request: %w", c.errPrefix, err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	if c.auth != nil {
		c.auth.Authenticate(req)
	}

	return req, nil
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("%s: encoding %s %s request: %w", c.errPrefix, method, path, err)
		}
		reader = bytes.NewReader(buf)
	}

	req, err := c.newRequest(ctx, method, path, reader)
	if err != nil {
		return err
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %s %s: %w", c.errPrefix, method, path, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if err := c.checkResponse(resp, method, path); err != nil {
		return err
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil // success with an empty body
		}

		return fmt.Errorf("%s: decoding %s %s response: %w", c.errPrefix, method, path, err)
	}

	return nil
}

// GetBinary performs a GET request and returns the raw response body. The
// caller must close the returned ReadCloser. The second return value is
// the response Content-Type.
func (c *Client) GetBinary(ctx context.Context, path string) (io.ReadCloser, string, error) {
	req, err := c.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, "", err
	}

	req.Header.Del("Accept")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("%s: GET %s: %w", c.errPrefix, path, err)
	}

	if err := c.checkResponse(resp, http.MethodGet, path); err != nil {
		if err := resp.Body.Close(); err != nil {
			return nil, "", err
		}

		return nil, "", err
	}

	return resp.Body, resp.Header.Get("Content-Type"), nil
}

// MultipartFile is one file part of a multipart upload.
type MultipartFile struct {
	Field    string
	Filename string
	Reader   io.Reader
}

// PostMultipart performs a multipart/form-data POST with the given form
// fields and files, streaming the request body.
func (c *Client) PostMultipart(ctx context.Context, path string, fields map[string]string, files []MultipartFile, out any) error {
	pr, pw := io.Pipe()
	mw := multipart.NewWriter(pw)

	go func() {
		err := func() error {
			for key, value := range fields {
				if err := mw.WriteField(key, value); err != nil {
					return err
				}
			}

			for _, f := range files {
				part, err := mw.CreateFormFile(f.Field, f.Filename)
				if err != nil {
					return err
				}

				if _, err := io.Copy(part, f.Reader); err != nil {
					return err
				}
			}

			return mw.Close()
		}()

		pw.CloseWithError(err)
	}()

	req, err := c.newRequest(ctx, http.MethodPost, path, pr)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s: POST %s: %w", c.errPrefix, path, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if err := c.checkResponse(resp, http.MethodPost, path); err != nil {
		return err
	}

	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}

		return fmt.Errorf("%s: decoding POST %s response: %w", c.errPrefix, path, err)
	}

	return nil
}
