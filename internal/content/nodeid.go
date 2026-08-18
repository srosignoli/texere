package content

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const shortIDAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
const shortIDLen = 8

// NodeID computes the stable node identifier from the node's system-assigned
// short ID.
//
// If shortID is non-empty, it returns shortID unchanged — the identity key is
// the short ID and nothing else, so it survives the collection being moved or
// copied to a new path.
// If shortID is empty, it returns "" — the node is untracked and cannot be scheduled.
//
// NOTE: @label values must NOT be passed as shortID. Labels are cross-reference
// anchors only and play no role in node identity. The shortID parameter must be
// the value parsed from the node's "# @id" marker line.
func NodeID(shortID string) string {
	return shortID
}

// newShortID is the source of new short IDs. It exists as a variable so tests
// can make issuance deterministic — a uniqueness guarantee cannot be tested
// against a random source, because a collision that the code fails to prevent
// is improbable rather than impossible and the test would pass on luck.
// Production code never reassigns it.
var newShortID = GenerateShortID

// GenerateShortID produces a cryptographically random 8-character base-36 string
// from the alphabet [0-9a-z].
func GenerateShortID() (string, error) {
	n := big.NewInt(int64(len(shortIDAlphabet)))
	b := make([]byte, shortIDLen)
	for i := range b {
		idx, err := rand.Int(rand.Reader, n)
		if err != nil {
			return "", fmt.Errorf("content: generate short ID: %w", err)
		}
		b[i] = shortIDAlphabet[idx.Int64()]
	}
	return string(b), nil
}
