package bot

import (
	"crypto/rand"
	"encoding/hex"
	"os"
)

// osReadDir is a thin wrapper around os.ReadDir.
func osReadDir(dir string) ([]os.DirEntry, error) { return os.ReadDir(dir) }

// randHex returns n random hex characters.
func randHex(n int) string {
	b := make([]byte, (n+1)/2)
	if _, err := rand.Read(b); err != nil {
		for i := range b {
			b[i] = 0xcd
		}
	}
	return hex.EncodeToString(b)[:n]
}
