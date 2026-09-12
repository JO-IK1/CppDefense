package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

const tokenBytes = 32

func NewToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func HashToken(key []byte, token string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(token))
	return mac.Sum(nil)
}

func VerifyToken(key []byte, token string, expected []byte) bool {
	return hmac.Equal(HashToken(key, token), expected)
}

func NewCSRFToken(key []byte, sessionID string) (string, []byte, error) {
	nonce, err := NewToken()
	if err != nil {
		return "", nil, err
	}
	token := sessionID + "." + nonce
	return token, HashToken(key, token), nil
}

func VerifyCSRFToken(key []byte, sessionID, token string, expected []byte) error {
	if !strings.HasPrefix(token, sessionID+".") {
		return errors.New("csrf token belongs to another session")
	}
	if !VerifyToken(key, token, expected) {
		return errors.New("invalid csrf token")
	}
	return nil
}
