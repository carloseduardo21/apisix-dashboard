// Package secure holds the small set of cryptographic helpers used by the
// dashboard: at-rest encryption of secrets and opaque random tokens.
package secure

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const KeySize = 32

// LoadKey reads a 32-byte key stored raw, hex or base64 encoded.
// When create is true and the file does not exist, a new key is generated.
func LoadKey(path string, create bool) (key []byte, created bool, err error) {
	b, err := os.ReadFile(path) // #nosec G304 -- key path comes from operator config
	if errors.Is(err, os.ErrNotExist) && create {
		key = make([]byte, KeySize)
		rand.Read(key)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, false, err
		}
		if err := os.WriteFile(path, []byte(hex.EncodeToString(key)+"\n"), 0o600); err != nil {
			return nil, false, err
		}
		return key, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	key, err = parseKey(b)
	return key, false, err
}

func parseKey(b []byte) ([]byte, error) {
	s := strings.TrimSpace(string(b))
	if k, err := hex.DecodeString(s); err == nil && len(k) == KeySize {
		return k, nil
	}
	if k, err := base64.StdEncoding.DecodeString(s); err == nil && len(k) == KeySize {
		return k, nil
	}
	if len(b) == KeySize {
		return b, nil
	}
	return nil, fmt.Errorf("encryption key must be %d bytes (raw, hex or base64)", KeySize)
}

// AEAD encrypts small values with AES-256-GCM. The output is nonce||ciphertext.
type AEAD struct{ gcm cipher.AEAD }

func NewAEAD(key []byte) (*AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &AEAD{gcm: gcm}, nil
}

// Seal encrypts plain. The additional data binds the ciphertext to its owner
// (e.g. a user id) so it cannot be copied to another record.
func (a *AEAD) Seal(plain, ad []byte) []byte {
	nonce := make([]byte, a.gcm.NonceSize())
	rand.Read(nonce)
	return a.gcm.Seal(nonce, nonce, plain, ad)
}

func (a *AEAD) Open(data, ad []byte) ([]byte, error) {
	n := a.gcm.NonceSize()
	if len(data) < n {
		return nil, errors.New("ciphertext too short")
	}
	return a.gcm.Open(nil, data[:n], data[n:], ad)
}

// RandomToken returns n random bytes encoded as unpadded base64url.
func RandomToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken is used to store session tokens: a database leak does not expose
// usable tokens.
func HashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}
