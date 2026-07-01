package authpassword

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

const cost = 12

// Hash returns a deliberately slow, salted password hash suitable for storage.
func Hash(password string) (string, error) {
	if password == "" {
		return "", errors.New("password must not be empty")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// Verify compares a plaintext password with a bcrypt hash.
func Verify(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// IsHash reports whether value is an encoded bcrypt password hash.
func IsHash(value string) bool {
	_, err := bcrypt.Cost([]byte(value))
	return err == nil
}
