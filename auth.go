package restage

import (
	"net/http"
	"sync"
)

// Authenticator applies authentication to an outgoing request. Implement
// it to support Bearer tokens, API-key headers, signed requests, and so
// on.
type Authenticator interface {
	Authenticate(req *http.Request)
}

// AuthenticatorFunc adapts a plain function to the Authenticator
// interface.
type AuthenticatorFunc func(req *http.Request)

// Authenticate calls f(req).
func (f AuthenticatorFunc) Authenticate(req *http.Request) { f(req) }

// BearerToken authenticates requests with an "Authorization: Bearer"
// header. The token may be replaced with SetToken (for example after a
// login); SetToken, Token, and Authenticate are safe for concurrent use.
type BearerToken struct {
	mu    sync.RWMutex
	token string
}

// NewBearerToken returns a BearerToken seeded with token (which may be
// empty).
func NewBearerToken(token string) *BearerToken {
	return &BearerToken{token: token}
}

// Authenticate adds the Authorization header when a token is set.
func (b *BearerToken) Authenticate(req *http.Request) {
	b.mu.RLock()
	token := b.token
	b.mu.RUnlock()

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// Token returns the current token.
func (b *BearerToken) Token() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.token
}

// SetToken replaces the token used for subsequent requests.
func (b *BearerToken) SetToken(token string) {
	b.mu.Lock()
	b.token = token
	b.mu.Unlock()
}
