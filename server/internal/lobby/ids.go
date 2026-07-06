package lobby

import (
	"crypto/rand"
	"fmt"
)

// joinCodeAlphabet excludes visually ambiguous characters (0/O, 1/I) so codes
// are easy to read aloud and type back in.
const joinCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

const joinCodeLength = 6

// newJoinCode generates a random 6-character join code from joinCodeAlphabet
// using a CSPRNG (Design_suite 6.2: private rooms are joined by code).
func newJoinCode() (string, error) {
	buf := make([]byte, joinCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, joinCodeLength)
	for i, b := range buf {
		out[i] = joinCodeAlphabet[int(b)%len(joinCodeAlphabet)]
	}
	return string(out), nil
}

// newTableID generates a random opaque table identifier. Not a full UUIDv4
// implementation (no external dependency); format is compatible enough
// (128 bits of randomness, hyphenated) for internal use as a unique ID.
func newTableID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	// Set version (4) and variant (RFC 4122) bits for a conventional look.
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}
