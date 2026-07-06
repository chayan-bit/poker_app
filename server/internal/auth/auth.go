// Package auth handles identity: guest device tokens that can upgrade to full
// accounts (see Design_suite 6.1). Guests can play from a link immediately; the
// upgrade path preserves their chips and history.
//
// This is a scaffold: swap the in-memory store for PostgreSQL before
// production. Tokens are HMAC-SHA256 signed and verified before any identity
// is trusted; never trust client-supplied identity without checking the
// signature.
package auth

import (
	"errors"
	"net/http"
	"time"
)

var ErrNoToken = errors.New("missing or invalid auth token")

// GuestTokenTTL is how long a guest cookie/token stays valid.
const GuestTokenTTL = 30 * 24 * time.Hour

// AccountTokenTTL is how long an upgraded-account token stays valid.
const AccountTokenTTL = 30 * 24 * time.Hour

// Identity is a resolved caller.
type Identity struct {
	PlayerID string
	Guest    bool
}

// Authenticator resolves an HTTP request to an Identity by verifying signed
// tokens against Store-backed accounts.
type Authenticator struct {
	tokens *TokenIssuer
	store  Store
}

// NewAuthenticator builds an Authenticator backed by secret and store.
func NewAuthenticator(secret []byte, store Store) *Authenticator {
	return &Authenticator{tokens: NewTokenIssuer(secret), store: store}
}

// FromRequest extracts and verifies identity from an Authorization bearer
// token or a signed "guest" cookie. Returns ErrNoToken (wrapped) if neither is
// present or valid.
func (a *Authenticator) FromRequest(r *http.Request) (string, error) {
	if tok := bearer(r); tok != "" {
		id, err := a.tokens.VerifyToken(tok)
		if err != nil {
			return "", ErrNoToken
		}
		return id.PlayerID, nil
	}
	if ck, err := r.Cookie("guest"); err == nil && ck.Value != "" {
		id, err := a.tokens.VerifyToken(ck.Value)
		if err != nil {
			return "", ErrNoToken
		}
		return id.PlayerID, nil
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
