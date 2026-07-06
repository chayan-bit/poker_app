package auth

import (
	"strings"
	"testing"
	"time"
)

func testIssuer() *TokenIssuer {
	return NewTokenIssuer([]byte("test-secret-do-not-use-in-prod"))
}

func TestIssueAndVerifyRoundtrip(t *testing.T) {
	issuer := testIssuer()

	tok, err := issuer.Issue("player-1", false, time.Hour)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	id, err := issuer.VerifyToken(tok)
	if err != nil {
		t.Fatalf("VerifyToken returned error: %v", err)
	}
	if id.PlayerID != "player-1" {
		t.Errorf("PlayerID = %q, want %q", id.PlayerID, "player-1")
	}
	if id.Guest {
		t.Errorf("Guest = true, want false")
	}
}

func TestVerifyTokenRejectsTamperedSignature(t *testing.T) {
	issuer := testIssuer()

	tok, err := issuer.Issue("player-1", true, time.Hour)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	parts := strings.SplitN(tok, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("token does not have two parts: %q", tok)
	}
	tampered := parts[0] + "." + "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

	if _, err := issuer.VerifyToken(tampered); err == nil {
		t.Fatal("VerifyToken accepted a tampered signature")
	}
}

func TestVerifyTokenRejectsTamperedPayload(t *testing.T) {
	issuer := testIssuer()

	tok, err := issuer.Issue("player-1", true, time.Hour)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	parts := strings.SplitN(tok, ".", 2)
	tampered := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA." + parts[1]

	if _, err := issuer.VerifyToken(tampered); err == nil {
		t.Fatal("VerifyToken accepted a tampered payload")
	}
}

func TestVerifyTokenRejectsExpired(t *testing.T) {
	issuer := testIssuer()

	tok, err := issuer.Issue("player-1", false, -time.Hour)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	_, err = issuer.VerifyToken(tok)
	if err != ErrExpiredToken {
		t.Fatalf("VerifyToken error = %v, want ErrExpiredToken", err)
	}
}

func TestVerifyTokenRejectsMalformed(t *testing.T) {
	issuer := testIssuer()

	cases := []string{
		"",
		"no-dot-here",
		"onlyprefix.",
		"!!!not-base64!!!.also-not-base64",
	}
	for _, tok := range cases {
		if _, err := issuer.VerifyToken(tok); err == nil {
			t.Errorf("VerifyToken(%q) accepted malformed token", tok)
		}
	}
}

func TestVerifyTokenRejectsDifferentSecret(t *testing.T) {
	issuer := testIssuer()
	other := NewTokenIssuer([]byte("a-completely-different-secret"))

	tok, err := issuer.Issue("player-1", false, time.Hour)
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	if _, err := other.VerifyToken(tok); err == nil {
		t.Fatal("VerifyToken accepted a token signed with a different secret")
	}
}
