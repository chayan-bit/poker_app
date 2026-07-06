package ws

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/chayan-bit/poker_app/server/internal/auth"
	"github.com/chayan-bit/poker_app/server/internal/table"
)

// dialToken dials the gateway with a ?token= query parameter (the browser WS
// auth path) and the given origin, returning the handshake error (nil on
// success). Any opened connection is closed before returning.
func dialToken(t *testing.T, srv interface{ URLString() string }, token, origin string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts := &websocket.DialOptions{HTTPHeader: http.Header{"Origin": []string{origin}}}
	url := srv.URLString()
	if token != "" {
		url += "?token=" + token
	}
	c, _, err := websocket.Dial(ctx, url, opts)
	if err == nil {
		c.Close(websocket.StatusNormalClosure, "")
	}
	return err
}

// tokenServer wraps httptest with a wsURL accessor for dialToken.
type tokenServer struct{ url string }

func (s tokenServer) URLString() string { return s.url }

func newAuthGateway(t *testing.T) (*Gateway, *auth.TokenIssuer) {
	t.Helper()
	secret := []byte("ws-auth-test-secret")
	issuer := auth.NewTokenIssuer(secret)
	authn := auth.NewAuthenticator(secret, auth.NewMemStore())
	gw := &Gateway{
		Reg:  table.NewRegistry(),
		Auth: TokenFromQuery(authn.FromRequest),
	}
	return gw, issuer
}

func TestGateway_ValidQueryTokenAuthenticates(t *testing.T) {
	gw, issuer := newAuthGateway(t)
	srv := newTestServer(t, gw)

	tok, err := issuer.Issue("player-42", true, time.Hour)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	if err := dialToken(t, tokenServer{wsURL(srv)}, tok, "http://127.0.0.1:1"); err != nil {
		t.Fatalf("valid ?token= handshake should succeed, got: %v", err)
	}
}

func TestGateway_InvalidTokenRejected(t *testing.T) {
	gw, _ := newAuthGateway(t)
	srv := newTestServer(t, gw)

	if err := dialToken(t, tokenServer{wsURL(srv)}, "not-a-real-token", "http://127.0.0.1:1"); err == nil {
		t.Fatal("handshake with an invalid ?token= must be rejected")
	}
}

func TestGateway_MissingTokenRejected(t *testing.T) {
	gw, _ := newAuthGateway(t)
	srv := newTestServer(t, gw)

	if err := dialToken(t, tokenServer{wsURL(srv)}, "", "http://127.0.0.1:1"); err == nil {
		t.Fatal("handshake with no token must be rejected")
	}
}

func TestGateway_NilAuthFailsClosed(t *testing.T) {
	gw := &Gateway{Reg: table.NewRegistry()} // Auth deliberately unset
	srv := newTestServer(t, gw)

	if err := dialToken(t, tokenServer{wsURL(srv)}, "anything", "http://127.0.0.1:1"); err == nil {
		t.Fatal("a gateway with no Auth verifier must reject every connection")
	}
}

func TestTokenFromQuery_PromotesQueryParamToBearer(t *testing.T) {
	var gotAuthHeader string
	verify := func(r *http.Request) (string, error) {
		gotAuthHeader = r.Header.Get("Authorization")
		return "p1", nil
	}
	wrapped := TokenFromQuery(verify)

	r, _ := http.NewRequest(http.MethodGet, "/ws?token=abc", nil)
	if _, err := wrapped(r); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if gotAuthHeader != "Bearer abc" {
		t.Fatalf("query token not promoted to bearer header: got %q", gotAuthHeader)
	}
}

func TestTokenFromQuery_KeepsExistingAuthorizationHeader(t *testing.T) {
	var gotAuthHeader string
	verify := func(r *http.Request) (string, error) {
		gotAuthHeader = r.Header.Get("Authorization")
		return "p1", nil
	}
	wrapped := TokenFromQuery(verify)

	r, _ := http.NewRequest(http.MethodGet, "/ws?token=abc", nil)
	r.Header.Set("Authorization", "Bearer existing")
	if _, err := wrapped(r); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if gotAuthHeader != "Bearer existing" {
		t.Fatalf("existing Authorization header must be preserved, got %q", gotAuthHeader)
	}
}
