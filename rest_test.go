package rest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc, opts ...Option) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return New(srv.URL, opts...)
}

func TestNewTrimsBaseURL(t *testing.T) {
	c := New("https://example.com/")
	if c.BaseURL() != "https://example.com" {
		t.Errorf("BaseURL = %q", c.BaseURL())
	}
}

func TestDoDecodesAndSendsBody(t *testing.T) {
	var (
		gotMethod, gotCT, gotAuth, gotUA string
		gotBody                          []byte
	)

	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotCT = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotBody, _ = io.ReadAll(r.Body)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}, WithAuthenticator(NewBearerToken("tok")), WithUserAgent("kit/1"))

	var out struct {
		OK bool `json:"ok"`
	}

	if err := c.Post(context.Background(), "/x", map[string]string{"a": "b"}, &out); err != nil {
		t.Fatalf("Post: %v", err)
	}

	if gotMethod != http.MethodPost || gotCT != "application/json" {
		t.Errorf("method=%s ct=%s", gotMethod, gotCT)
	}

	if gotAuth != "Bearer tok" || gotUA != "kit/1" {
		t.Errorf("auth=%q ua=%q", gotAuth, gotUA)
	}

	if strings.TrimSpace(string(gotBody)) != `{"a":"b"}` {
		t.Errorf("body = %s", gotBody)
	}

	if !out.OK {
		t.Errorf("decoded = %+v", out)
	}
}

func TestDoNoAuthHeaderWhenUnset(t *testing.T) {
	var hadAuth bool
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		w.WriteHeader(http.StatusOK)
	})

	if err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}

	if hadAuth {
		t.Error("Authorization header set without an authenticator")
	}
}

func TestEmptyBodySuccess(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	var out struct{}
	if err := c.Get(context.Background(), "/x", &out); err != nil {
		t.Fatalf("Get with empty body: %v", err)
	}
}

func TestErrorResponseCarriesPrefixAndStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}, WithErrorPrefix("kit"))

	err := c.Get(context.Background(), "/x", nil)
	if !IsForbidden(err) {
		t.Fatalf("IsForbidden = false, err = %v", err)
	}

	var apiErr *Error
	if !HasStatus(err, http.StatusForbidden) {
		t.Fatal("HasStatus(403) = false")
	}

	if got := err.Error(); !strings.HasPrefix(got, "kit: GET /x: 403") {
		t.Errorf("Error() = %q", got)
	}

	_ = apiErr
}

func TestGetBinary(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "" {
			t.Errorf("Accept should be removed for binary GET, got %q", r.Header.Get("Accept"))
		}

		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-bytes"))
	})

	body, ct, err := c.GetBinary(context.Background(), "/cover")
	if err != nil {
		t.Fatalf("GetBinary: %v", err)
	}
	defer func() { _ = body.Close() }()

	if ct != "image/png" {
		t.Errorf("content-type = %q", ct)
	}

	if data, _ := io.ReadAll(body); string(data) != "png-bytes" {
		t.Errorf("body = %q", data)
	}
}

func TestPostMultipart(t *testing.T) {
	var gotField, gotFile string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse: %v", err)
		}

		gotField = r.FormValue("title")
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}

		defer func() { _ = f.Close() }()

		b, _ := io.ReadAll(f)
		gotFile = string(b)
		w.WriteHeader(http.StatusOK)
	})

	files := []MultipartFile{{Field: "file", Filename: "b.bin", Reader: strings.NewReader("data")}}
	if err := c.PostMultipart(context.Background(), "/upload", map[string]string{"title": "T"}, files, nil); err != nil {
		t.Fatalf("PostMultipart: %v", err)
	}

	if gotField != "T" || gotFile != "data" {
		t.Errorf("field=%q file=%q", gotField, gotFile)
	}
}

func TestPath(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"prefix", NewPath("/api").String(), "/api"},
		{"seg", NewPath("/api").Seg("lib_1").String(), "/api/lib_1"},
		{"lit then seg", NewPath("/api").Lit("libraries").Seg("lib_1").Lit("items").String(), "/api/libraries/lib_1/items"},
		{"escape", NewPath("/api").Seg("a b/c").String(), "/api/a%20b%2Fc"},
		{"skip empty seg", NewPath("/api").Seg("x", "").String(), "/api/x"},
		{"flag on", NewPath("/x").Flag("hard", true).String(), "/x?hard=1"},
		{"flag off", NewPath("/x").Flag("hard", false).String(), "/x"},
		{"set sorted", NewPath("/s").Set("b", "2").Set("a", "1").String(), "/s?a=1&b=2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestPathQueryMerge(t *testing.T) {
	q := url.Values{"limit": {"10"}, "page": {"2"}}
	if got := NewPath("/x").Query(q).String(); got != "/x?limit=10&page=2" {
		t.Errorf("got %q", got)
	}

	if got := NewPath("/x").Query(nil).String(); got != "/x" {
		t.Errorf("nil query: got %q", got)
	}
}

func TestBearerToken(t *testing.T) {
	b := NewBearerToken("")
	req, _ := http.NewRequest(http.MethodGet, "http://x", nil)
	b.Authenticate(req)

	if req.Header.Get("Authorization") != "" {
		t.Error("empty token should not set header")
	}

	b.SetToken("abc")
	if b.Token() != "abc" {
		t.Errorf("Token = %q", b.Token())
	}

	b.Authenticate(req)
	if req.Header.Get("Authorization") != "Bearer abc" {
		t.Errorf("Authorization = %q", req.Header.Get("Authorization"))
	}
}

func TestBearerTokenConcurrent(t *testing.T) {
	var wg sync.WaitGroup

	b := NewBearerToken("")

	for range 50 {
		wg.Add(3)

		go func() { defer wg.Done(); b.SetToken("tok") }()

		go func() { defer wg.Done(); _ = b.Token() }()

		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, "http://x", nil)
			b.Authenticate(req)
		}()
	}

	wg.Wait()
}

func TestAuthenticatorFunc(t *testing.T) {
	var called bool
	var a Authenticator = AuthenticatorFunc(func(*http.Request) { called = true })

	a.Authenticate(&http.Request{})

	if !called {
		t.Error("AuthenticatorFunc not invoked")
	}
}

func TestVerbs(t *testing.T) {
	var gotMethod string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	})

	ctx := context.Background()
	cases := []struct {
		name string
		call func() error
		want string
	}{
		{"Put", func() error { return c.Put(ctx, "/x", nil, nil) }, http.MethodPut},
		{"Patch", func() error { return c.Patch(ctx, "/x", nil, nil) }, http.MethodPatch},
		{"Delete", func() error { return c.Delete(ctx, "/x", nil) }, http.MethodDelete},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); err != nil {
				t.Fatalf("%s: %v", tt.name, err)
			}

			if gotMethod != tt.want {
				t.Errorf("method = %s, want %s", gotMethod, tt.want)
			}
		})
	}
}

func TestStatusHelpers(t *testing.T) {
	if !IsNotFound(&Error{StatusCode: http.StatusNotFound}) {
		t.Error("IsNotFound")
	}

	if !IsUnauthorized(&Error{StatusCode: http.StatusUnauthorized}) {
		t.Error("IsUnauthorized")
	}

	if !IsBadRequest(&Error{StatusCode: http.StatusBadRequest}) {
		t.Error("IsBadRequest")
	}
}

func TestConfigOptions(t *testing.T) {
	// WithHTTPClient, WithTimeout, and WithInsecureSkipVerify must apply
	// without breaking a request.
	custom := &http.Client{}
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}, WithHTTPClient(custom), WithTimeout(2*time.Second), WithInsecureSkipVerify())

	if err := c.Get(context.Background(), "/x", nil); err != nil {
		t.Fatalf("Get: %v", err)
	}

	if custom.Timeout != 2*time.Second {
		t.Errorf("WithTimeout not applied to custom client: %v", custom.Timeout)
	}

	if _, ok := custom.Transport.(*http.Transport); !ok {
		t.Error("WithInsecureSkipVerify did not set a *http.Transport")
	}
}
