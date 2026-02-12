// Package hexid generates short random hex identifiers.
package hexid

import (
	"crypto/rand"
	"encoding/hex"
)

// New returns an 8-character lowercase hex string (4 random bytes).
func New() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("hexid: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}
