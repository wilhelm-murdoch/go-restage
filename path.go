package rest

import (
	"net/url"
	"strings"
)

// Path assembles a request path together with its query string. Start one
// with NewPath, append dynamic, user-supplied values with Seg (each is
// escaped with url.PathEscape) and fixed sub-resource names with Lit
// (used verbatim), and add query parameters with Query, Set, or Flag.
// Render the result with String.
//
// Empty segments are skipped, so optional identifiers can be passed
// unconditionally.
type Path struct {
	b     strings.Builder
	query url.Values
}

// NewPath starts a builder at a trusted, verbatim prefix that is used as
// given, e.g. NewPath("/api") or NewPath("/login").
func NewPath(prefix string) *Path {
	p := &Path{}
	p.b.WriteString(prefix)
	return p
}

// Seg appends one escaped path segment per non-empty value. Values are
// escaped with url.PathEscape, so never pre-escape user input.
func (p *Path) Seg(segments ...string) *Path {
	for _, s := range segments {
		if s == "" {
			continue
		}

		p.b.WriteByte('/')
		p.b.WriteString(url.PathEscape(s))
	}
	return p
}

// Lit appends one verbatim path segment per non-empty value. Use it only
// for trusted, fixed sub-resource names such as "items" or "cover".
func (p *Path) Lit(segments ...string) *Path {
	for _, s := range segments {
		if s == "" {
			continue
		}

		p.b.WriteByte('/')
		p.b.WriteString(s)
	}
	return p
}

// Query merges the given values into the query string. A nil or empty q
// is a no-op.
func (p *Path) Query(q url.Values) *Path {
	for key, values := range q {
		for _, v := range values {
			p.add(key, v)
		}
	}

	return p
}

// Set adds a single query parameter.
func (p *Path) Set(key, value string) *Path {
	p.add(key, value)
	return p
}

// Flag adds key=1 when on, matching the common convention for boolean
// query parameters, and does nothing otherwise.
func (p *Path) Flag(key string, on bool) *Path {
	if on {
		p.add(key, "1")
	}

	return p
}

func (p *Path) add(key, value string) {
	if p.query == nil {
		p.query = url.Values{}
	}

	p.query.Add(key, value)
}

// String renders the full path, appending "?"+encoded query when any
// query parameters were set.
func (p *Path) String() string {
	if len(p.query) == 0 {
		return p.b.String()
	}

	return p.b.String() + "?" + p.query.Encode()
}
