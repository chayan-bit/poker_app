package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testAuthenticator() *Authenticator {
	return NewAuthenticator([]byte("handlers-test-secret"), NewMemStore())
}

func TestFromRequestBearerToken(t *testing.T) {
	a := testAuthenticator()

	rec := httptest.NewRecorder()
	a.GuestHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/guest", nil))
	var resp guestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode guest response: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+resp.Token)

	playerID, err := a.FromRequest(req)
	if err != nil {
		t.Fatalf("FromRequest returned error: %v", err)
	}
	if playerID != resp.PlayerID {
		t.Errorf("playerID = %q, want %q", playerID, resp.PlayerID)
	}
}

func TestFromRequestGuestCookieFlow(t *testing.T) {
	a := testAuthenticator()

	rec := httptest.NewRecorder()
	a.GuestHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/guest", nil))
	var resp guestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode guest response: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.AddCookie(&http.Cookie{Name: "guest", Value: resp.Token})

	playerID, err := a.FromRequest(req)
	if err != nil {
		t.Fatalf("FromRequest returned error: %v", err)
	}
	if playerID != resp.PlayerID {
		t.Errorf("playerID = %q, want %q", playerID, resp.PlayerID)
	}
}

func TestFromRequestNoTokenReturnsErrNoToken(t *testing.T) {
	a := testAuthenticator()

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if _, err := a.FromRequest(req); err != ErrNoToken {
		t.Fatalf("FromRequest error = %v, want ErrNoToken", err)
	}
}

func TestFromRequestRejectsRawUnsignedCookie(t *testing.T) {
	a := testAuthenticator()

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.AddCookie(&http.Cookie{Name: "guest", Value: "just-some-raw-value"})

	if _, err := a.FromRequest(req); err != ErrNoToken {
		t.Fatalf("FromRequest error = %v, want ErrNoToken for raw unsigned cookie", err)
	}
}

func TestGuestHandlerReturnsToken(t *testing.T) {
	a := testAuthenticator()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/guest", nil)
	a.GuestHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp guestResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Token == "" || resp.PlayerID == "" {
		t.Fatalf("expected non-empty token and playerID, got %+v", resp)
	}

	id, err := a.tokens.VerifyToken(resp.Token)
	if err != nil {
		t.Fatalf("issued token failed verification: %v", err)
	}
	if !id.Guest {
		t.Error("expected guest token to have Guest=true")
	}
}

func TestGuestHandlerRejectsNonPost(t *testing.T) {
	a := testAuthenticator()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/guest", nil)
	a.GuestHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestUpgradeHandlerPreservesPlayerIDAndReturnsFreshToken(t *testing.T) {
	a := testAuthenticator()

	rec := httptest.NewRecorder()
	a.GuestHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/guest", nil))
	var guestResp guestResponse
	if err := json.NewDecoder(rec.Body).Decode(&guestResp); err != nil {
		t.Fatalf("decode guest response: %v", err)
	}

	body, _ := json.Marshal(upgradeRequest{Email: "chayan@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/upgrade", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+guestResp.Token)

	rec2 := httptest.NewRecorder()
	a.UpgradeHandler().ServeHTTP(rec2, req)

	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body=%s", rec2.Code, http.StatusOK, rec2.Body.String())
	}

	var resp upgradeResponse
	if err := json.NewDecoder(rec2.Body).Decode(&resp); err != nil {
		t.Fatalf("decode upgrade response: %v", err)
	}
	if resp.PlayerID != guestResp.PlayerID {
		t.Errorf("PlayerID = %q, want %q (must be preserved)", resp.PlayerID, guestResp.PlayerID)
	}
	if resp.Email != "chayan@example.com" {
		t.Errorf("Email = %q, want %q", resp.Email, "chayan@example.com")
	}

	id, err := a.tokens.VerifyToken(resp.Token)
	if err != nil {
		t.Fatalf("returned token failed verification: %v", err)
	}
	if id.Guest {
		t.Error("expected upgraded token to have Guest=false")
	}
	if id.PlayerID != guestResp.PlayerID {
		t.Errorf("token PlayerID = %q, want %q", id.PlayerID, guestResp.PlayerID)
	}
}

func TestUpgradeHandlerRejectsUnauthenticated(t *testing.T) {
	a := testAuthenticator()

	body, _ := json.Marshal(upgradeRequest{Email: "nobody@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/auth/upgrade", bytes.NewReader(body))

	rec := httptest.NewRecorder()
	a.UpgradeHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestUpgradeHandlerRejectsDuplicateEmail(t *testing.T) {
	a := testAuthenticator()

	issueGuestToken := func() guestResponse {
		rec := httptest.NewRecorder()
		a.GuestHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/guest", nil))
		var resp guestResponse
		_ = json.NewDecoder(rec.Body).Decode(&resp)
		return resp
	}

	first := issueGuestToken()
	body1, _ := json.Marshal(upgradeRequest{Email: "dup@example.com"})
	req1 := httptest.NewRequest(http.MethodPost, "/api/auth/upgrade", bytes.NewReader(body1))
	req1.Header.Set("Authorization", "Bearer "+first.Token)
	rec1 := httptest.NewRecorder()
	a.UpgradeHandler().ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first upgrade status = %d, want %d, body=%s", rec1.Code, http.StatusOK, rec1.Body.String())
	}

	second := issueGuestToken()
	body2, _ := json.Marshal(upgradeRequest{Email: "dup@example.com"})
	req2 := httptest.NewRequest(http.MethodPost, "/api/auth/upgrade", bytes.NewReader(body2))
	req2.Header.Set("Authorization", "Bearer "+second.Token)
	rec2 := httptest.NewRecorder()
	a.UpgradeHandler().ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusConflict {
		t.Fatalf("second upgrade status = %d, want %d", rec2.Code, http.StatusConflict)
	}
}

func TestUpgradeHandlerRejectsMissingEmail(t *testing.T) {
	a := testAuthenticator()

	rec := httptest.NewRecorder()
	a.GuestHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/auth/guest", nil))
	var guestResp guestResponse
	_ = json.NewDecoder(rec.Body).Decode(&guestResp)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/upgrade", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Authorization", "Bearer "+guestResp.Token)

	rec2 := httptest.NewRecorder()
	a.UpgradeHandler().ServeHTTP(rec2, req)

	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec2.Code, http.StatusBadRequest)
	}
}
