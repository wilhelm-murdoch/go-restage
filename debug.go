package restage

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// maxDebugBody caps how much of a body is printed. The full body is always
// preserved for the request/response; only the logged copy is truncated.
const maxDebugBody = 8 << 10

// sensitiveHeaders are redacted in debug output so credentials never reach
// the log.
var sensitiveHeaders = map[string]bool{
	"Authorization":       true,
	"Proxy-Authorization": true,
	"Cookie":              true,
	"Set-Cookie":          true,
}

// WithDebug logs every request and response (method, URL, headers, and
// body) to w. It is a debugging aid, not a production logger.
//
// Security: bodies are printed as-is and may contain secrets - most
// notably the username and password sent by Login. The Authorization and
// cookie headers are redacted, but request/response bodies are not, so
// never enable this against a server reachable over an untrusted network
// and always scrub debug output before sharing it.
func WithDebug(w io.Writer) Option {
	return func(c *Client) { c.debug = w }
}

// debugTransport wraps a RoundTripper and writes a human-readable trace of
// each exchange.
type debugTransport struct {
	next http.RoundTripper
	w    io.Writer
}

func newDebugTransport(next http.RoundTripper, w io.Writer) *debugTransport {
	return &debugTransport{next: next, w: w}
}

// printf writes best-effort debug output; write errors to the debug sink
// are intentionally ignored.
func (d *debugTransport) printf(format string, args ...any) {
	_, _ = fmt.Fprintf(d.w, format, args...)
}

func (d *debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	d.printf(">> %s %s\n", req.Method, req.URL)
	d.writeHeaders(req.Header)
	d.logRequestBody(req)

	start := time.Now()
	resp, err := d.next.RoundTrip(req)
	elapsed := time.Since(start).Round(time.Millisecond)

	if err != nil {
		d.printf("<< error after %s: %v\n\n", elapsed, err)
		return resp, err
	}

	d.printf("<< %s (%s)\n", resp.Status, elapsed)
	d.writeHeaders(resp.Header)
	d.logResponseBody(resp)
	d.printf("\n")

	return resp, nil
}

// logRequestBody prints the request body, restoring it so the request can
// still be sent. Multipart uploads are skipped to preserve streaming.
func (d *debugTransport) logRequestBody(req *http.Request) {
	if req.Body == nil || req.Body == http.NoBody {
		return
	}

	if strings.HasPrefix(req.Header.Get("Content-Type"), "multipart/") {
		d.printf("   body: <multipart form omitted>\n")
		return
	}

	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		d.printf("   body: <error reading request body: %v>\n", err)
		return
	}

	req.Body = io.NopCloser(bytes.NewReader(body))
	if req.GetBody == nil {
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}

	d.printf("   body: %s\n", debugBody(body))
}

// logResponseBody prints the response body, restoring it so the caller can
// still decode it. Non-textual bodies (images, downloads) are skipped.
func (d *debugTransport) logResponseBody(resp *http.Response) {
	if resp.Body == nil || resp.Body == http.NoBody {
		return
	}

	if !isTextual(resp.Header.Get("Content-Type")) {
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = "binary"
		}

		d.printf("   body: <%s omitted>\n", ct)

		return
	}

	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		d.printf("   body: <error reading response body: %v>\n", err)
		resp.Body = io.NopCloser(bytes.NewReader(body))
		return
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	d.printf("   body: %s\n", debugBody(body))
}

func (d *debugTransport) writeHeaders(h http.Header) {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		value := strings.Join(h[k], ", ")
		if sensitiveHeaders[http.CanonicalHeaderKey(k)] {
			value = "<redacted>"
		}

		d.printf("   %s: %s\n", k, value)
	}
}

// isTextual reports whether a Content-Type is safe to print as text.
func isTextual(contentType string) bool {
	if contentType == "" {
		return true // assume textual (e.g. small JSON without a header)
	}

	mediaType := strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	switch {
	case strings.HasPrefix(mediaType, "text/"):
		return true
	case mediaType == "application/json", mediaType == "application/xml", mediaType == "application/xhtml+xml":
		return true
	case strings.HasSuffix(mediaType, "+json"), strings.HasSuffix(mediaType, "+xml"):
		return true
	default:
		return false
	}
}

// debugBody returns the body as a string, a placeholder when it contains
// binary data that would corrupt terminal output, or a truncated copy when
// it is large.
func debugBody(b []byte) string {
	if len(b) == 0 {
		return "<empty>"
	}

	if !utf8.Valid(b) || bytes.IndexByte(b, 0) != -1 {
		return "<skipping binary output>"
	}

	if len(b) > maxDebugBody {
		// Back up to a rune boundary so the cut never splits a multibyte
		// character; b is known to be valid UTF-8 here.
		cut := maxDebugBody
		for cut > 0 && !utf8.RuneStart(b[cut]) { //nolint:gosec // G602: cut <= maxDebugBody < len(b), guarded above
			cut--
		}

		return string(b[:cut]) + fmt.Sprintf("… (%d more bytes)", len(b)-cut)
	}

	return string(b)
}
