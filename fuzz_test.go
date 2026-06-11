package restage

import (
	"bytes"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzPathSeg guards the Path builder's security boundary: anything passed
// through Seg must survive a URL parse round-trip intact and must never be
// able to smuggle in extra path segments, a query string, or a fragment.
func FuzzPathSeg(f *testing.F) {
	f.Add("simple", "key", "value")
	f.Add("a b/c", "q", "x y&z")
	f.Add("../../../etc/passwd", "", "")
	f.Add("seg?injected=1#frag", "key", "%2F%00")
	f.Add("ünïcødé/⛳", "ключ", "значение")

	f.Fuzz(func(t *testing.T, seg, key, val string) {
		p := NewPath("/api").Seg(seg)
		if key != "" {
			p.Set(key, val)
		}
		rendered := p.String()

		u, err := url.Parse(rendered)
		if err != nil {
			t.Fatalf("rendered path %q does not parse: %v", rendered, err)
		}

		if seg != "" {
			escaped := strings.TrimPrefix(u.EscapedPath(), "/api/")
			if strings.ContainsAny(escaped, "/?#") {
				t.Fatalf("segment %q escaped to %q, which can inject path/query/fragment", seg, escaped)
			}

			roundTripped, err := url.PathUnescape(escaped)
			if err != nil {
				t.Fatalf("unescape %q: %v", escaped, err)
			}
			if roundTripped != seg {
				t.Fatalf("segment round-trip: got %q, want %q", roundTripped, seg)
			}
		}

		if key != "" {
			if got := u.Query().Get(key); got != val {
				t.Fatalf("query round-trip for key %q: got %q, want %q", key, got, val)
			}
		}
	})
}

// FuzzDebugBody guards the debug-trace body formatter: output goes straight
// to a terminal, so it must stay bounded, remain valid UTF-8, and never
// contain NUL bytes regardless of input.
func FuzzDebugBody(f *testing.F) {
	f.Add([]byte(nil))
	f.Add([]byte("plain text"))
	f.Add([]byte{0x00})
	f.Add([]byte{0xff, 0xfe, 0xfd})
	f.Add(bytes.Repeat([]byte("a"), maxDebugBody+1))
	// A multibyte rune straddling the truncation boundary.
	f.Add(append(bytes.Repeat([]byte("a"), maxDebugBody-1), []byte("é⛳ tail")...))

	f.Fuzz(func(t *testing.T, b []byte) {
		out := debugBody(b)

		// maxDebugBody of body plus the "… (N more bytes)" suffix.
		if len(out) > maxDebugBody+64 {
			t.Fatalf("output exceeds bound: %d bytes", len(out))
		}
		if !utf8.ValidString(out) {
			t.Fatalf("output is not valid UTF-8: %q", out)
		}
		if strings.IndexByte(out, 0) != -1 {
			t.Fatalf("output contains NUL byte: %q", out)
		}
	})
}
