package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

type TokenHasher struct {
	secret []byte
}

func NewTokenHasher(secret string) TokenHasher {
	return TokenHasher{secret: []byte(secret)}
}

func (h TokenHasher) Hash(token string) string {
	mac := hmac.New(sha256.New, h.secret)
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}
