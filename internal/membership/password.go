// internal/membership/password.go
package membership

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"golang.org/x/crypto/argon2"
)

// hashPassword generates a salted Argon2id hash of the password.
func hashPassword(password string) (string, string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", "", err
	}

	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)

	encodedHash := base64.StdEncoding.EncodeToString(hash)
	encodedSalt := base64.StdEncoding.EncodeToString(salt)

	return encodedHash, encodedSalt, nil
}

// verifyPassword compares a password with a salted hash.
func verifyPassword(password, salt, hash string) (bool, error) {
	decodedSalt, err := base64.StdEncoding.DecodeString(salt)
	if err != nil {
		return false, fmt.Errorf("failed to decode salt: %w", err)
	}

	decodedHash, err := base64.StdEncoding.DecodeString(hash)
	if err != nil {
		return false, fmt.Errorf("failed to decode hash: %w", err)
	}

	comparisonHash := argon2.IDKey([]byte(password), decodedSalt, 1, 64*1024, 4, 32)

	return string(decodedHash) == string(comparisonHash), nil
}
