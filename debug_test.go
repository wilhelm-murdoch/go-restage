package rest

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDebugTransportLogsRedactsAndRestores(t *testing.T) {
	var buf bytes.Buffer

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The server must still receive the (restored) request body.
		body, _ := io.ReadAll(r.Body)
		if strings.TrimSpace(string(body)) != `{"q":"x"}` {
			t.Errorf("server received body %q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, WithDebug(&buf), WithAuthenticator(NewBearerToken("supersecret")))

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.Post(context.Background(), "/x", map[string]string{"q": "x"}, &out); err != nil {
		t.Fatalf("Post: %v", err)
	}

	// The response body must survive logging and decode normally.
	if !out.OK {
		t.Error("response body was not restored for the caller")
	}

	s := buf.String()
	for _, want := range []string{">> POST", `{"q":"x"}`, "<< 200", `{"ok":true}`, "<redacted>"} {
		if !strings.Contains(s, want) {
			t.Errorf("debug output missing %q\n---\n%s", want, s)
		}
	}
	if strings.Contains(s, "supersecret") {
		t.Errorf("bearer token leaked into debug output:\n%s", s)
	}
}

func TestDebugTransportSkipsBinaryResponse(t *testing.T) {
	var buf bytes.Buffer
	payload := []byte{0x89, 0x50, 0x4e, 0x47, 0x00, 0x01, 0x02}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, WithDebug(&buf))
	body, ct, err := c.GetBinary(context.Background(), "/cover")
	if err != nil {
		t.Fatalf("GetBinary: %v", err)
	}
	defer func() { _ = body.Close() }()

	got, _ := io.ReadAll(body)
	if !bytes.Equal(got, payload) || ct != "image/png" {
		t.Errorf("binary body corrupted: %v %q", got, ct)
	}

	if !strings.Contains(buf.String(), "<image/png omitted>") {
		t.Errorf("expected binary body to be omitted:\n%s", buf.String())
	}
}

func TestDebugTransportSkipsMultipart(t *testing.T) {
	var buf bytes.Buffer

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			t.Fatalf("form file: %v", err)
		}
		defer func() { _ = f.Close() }()
		if b, _ := io.ReadAll(f); string(b) != "filedata" {
			t.Errorf("server received file %q", b)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL, WithDebug(&buf))
	files := []MultipartFile{{Field: "file", Filename: "f.bin", Reader: strings.NewReader("filedata")}}
	if err := c.PostMultipart(context.Background(), "/upload", nil, files, nil); err != nil {
		t.Fatalf("PostMultipart: %v", err)
	}

	s := buf.String()
	if !strings.Contains(s, "<multipart form omitted>") {
		t.Errorf("multipart body should be omitted:\n%s", s)
	}
	if strings.Contains(s, "filedata") {
		t.Errorf("multipart file contents leaked into debug output:\n%s", s)
	}
}

func TestDebugTransportLogsError(t *testing.T) {
	var buf bytes.Buffer
	c := New("http://127.0.0.1:0", WithDebug(&buf)) // unroutable → transport error

	err := c.Get(context.Background(), "/x", nil)
	if err == nil {
		t.Fatal("expected a transport error")
	}
	if !strings.Contains(buf.String(), "<< error") {
		t.Errorf("transport error not logged:\n%s", buf.String())
	}
}

func TestDebugBody(t *testing.T) {
	if got := debugBody(nil); got != "<empty>" {
		t.Errorf("empty = %q", got)
	}
	if got := debugBody([]byte{0x00, 0x01}); got != "<skipping binary output>" {
		t.Errorf("binary = %q", got)
	}
	if got := debugBody([]byte("hello")); got != "hello" {
		t.Errorf("text = %q", got)
	}

	big := bytes.Repeat([]byte("a"), maxDebugBody+100)
	got := debugBody(big)
	if !strings.HasPrefix(got, strings.Repeat("a", 64)) || !strings.Contains(got, "more bytes)") {
		t.Errorf("large body not truncated: %.80q…", got)
	}
}

func TestIsTextual(t *testing.T) {
	textual := []string{"", "application/json", "application/json; charset=utf-8", "text/plain", "application/vnd.api+json", "application/xml"}
	for _, ct := range textual {
		if !isTextual(ct) {
			t.Errorf("isTextual(%q) = false, want true", ct)
		}
	}

	binary := []string{"image/png", "application/octet-stream", "audio/mpeg", "application/zip"}
	for _, ct := range binary {
		if isTextual(ct) {
			t.Errorf("isTextual(%q) = true, want false", ct)
		}
	}
}
