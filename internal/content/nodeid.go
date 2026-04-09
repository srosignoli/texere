package content

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

const shortIDAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
const shortIDLen = 8

// NodeID computes the stable node identifier from its file path, chapter title,
// and system-assigned short ID.
//
// If shortID is non-empty, it returns "<filepath>:<chapterTitle>:id:<shortID>".
// If shortID is empty, it returns "" — the node is untracked and cannot be scheduled.
//
// NOTE: @label values must NOT be passed as shortID. Labels are cross-reference
// anchors only and play no role in node identity. The shortID parameter must be
// the value parsed from the node's "# @id" marker line.
func NodeID(filepath, chapterTitle, shortID string) string {
	if shortID == "" {
		return ""
	}
	return filepath + ":" + chapterTitle + ":id:" + shortID
}

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
