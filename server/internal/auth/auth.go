// Package auth handles identity: guest device tokens that can upgrade to full
// accounts (see Design_suite 6.1). Guests can play from a link immediately; the
// upgrade path preserves their chips and history.
//
// This is a scaffold: swap the in-memory store for PostgreSQL and validate real
// signed tokens (JWT or paseto) before production. Never trust client-supplied
// identity without verifying the token signature.
package auth

import (
	"errors"
	"net/http"
)

var ErrNoToken = errors.New("missing or invalid auth token")

// Identity is a resolved caller.
type Identity struct {
	PlayerID string
	Guest    bool
}

// Authenticator resolves an HTTP request to an Identity.
type Authenticator struct {
	// TODO: inject a token verifier and a user repository.
}

// FromRequest extracts and verifies identity from an Authorization bearer token
// or a guest cookie. Placeholder logic returns the token value as the player ID.
func (a *Authenticator) FromRequest(r *http.Request) (string, error) {
	if tok := bearer(r); tok != "" {
		// TODO: verify signature, look up account, return canonical player ID.
		return tok, nil
	}
	if ck, err := r.Cookie("guest"); err == nil && ck.Value != "" {
		return "guest:" + ck.Value, nil
	}
	return "", ErrNoToken
}

func bearer(r *http.Request) string {
	const p = "Bearer "
	h := r.Header.Get("Authorization")
	if len(h) > len(p) && h[:len(p)] == p {
		return h[len(p):]
	}
	return ""
}
