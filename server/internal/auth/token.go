package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

// tokenPayload is the signed body of a token.
type tokenPayload struct {
	PlayerID string `json:"playerId"`
	Guest    bool   `json:"guest"`
	IAT      int64  `json:"iat"`
	EXP      int64  `json:"exp"`
}

// TokenIssuer signs and verifies identity tokens with an HMAC-SHA256 secret.
type TokenIssuer struct {
	secret []byte
}

// NewTokenIssuer builds a TokenIssuer from a secret. The secret must be
// non-empty; callers are responsible for keeping it stable across restarts
// (otherwise previously issued tokens stop verifying).
func NewTokenIssuer(secret []byte) *TokenIssuer {
	return &TokenIssuer{secret: secret}
}

// Issue mints a signed token for playerID valid for ttl.
func (t *TokenIssuer) Issue(playerID string, guest bool, ttl time.Duration) (string, error) {
	now := time.Now()
	payload := tokenPayload{
		PlayerID: playerID,
		Guest:    guest,
		IAT:      now.Unix(),
		EXP:      now.Add(ttl).Unix(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encBody := base64.RawURLEncoding.EncodeToString(body)
	sig := t.sign([]byte(encBody))
	encSig := base64.RawURLEncoding.EncodeToString(sig)
	return encBody + "." + encSig, nil
}

// VerifyToken checks the signature and expiry of tok and returns the resolved
// Identity on success.
func (t *TokenIssuer) VerifyToken(tok string) (Identity, error) {
	dot := -1
	for i := 0; i < len(tok); i++ {
		if tok[i] == '.' {
			dot = i
			break
		}
	}
	if dot < 0 || dot == len(tok)-1 {
		return Identity{}, ErrInvalidToken
	}
	encBody, encSig := tok[:dot], tok[dot+1:]

	sig, err := base64.RawURLEncoding.DecodeString(encSig)
	if err != nil {
		return Identity{}, ErrInvalidToken
	}
	expectedSig := t.sign([]byte(encBody))
	if subtle.ConstantTimeCompare(sig, expectedSig) != 1 {
		return Identity{}, ErrInvalidToken
	}

	body, err := base64.RawURLEncoding.DecodeString(encBody)
	if err != nil {
		return Identity{}, ErrInvalidToken
	}
	var payload tokenPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return Identity{}, ErrInvalidToken
	}
	if payload.PlayerID == "" {
		return Identity{}, ErrInvalidToken
	}
	if time.Now().Unix() > payload.EXP {
		return Identity{}, ErrExpiredToken
	}

	return Identity{PlayerID: payload.PlayerID, Guest: payload.Guest}, nil
}

func (t *TokenIssuer) sign(msg []byte) []byte {
	mac := hmac.New(sha256.New, t.secret)
	mac.Write(msg)
	return mac.Sum(nil)
}
