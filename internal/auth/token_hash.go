package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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

type TokenHashKeyRing struct {
	activeVersion int32
	hashers       map[int32]TokenHasher
}

func NewSingleTokenHashKeyRing(secret string) (*TokenHashKeyRing, error) {
	return NewTokenHashKeyRing(1, map[int32]string{1: secret})
}

func NewTokenHashKeyRing(activeVersion int32, secrets map[int32]string) (*TokenHashKeyRing, error) {
	if activeVersion <= 0 {
		return nil, fmt.Errorf("active token hash secret version must be greater than 0")
	}
	if len(secrets) == 0 {
		return nil, fmt.Errorf("token hash keyring must contain at least one secret")
	}
	hashers := make(map[int32]TokenHasher, len(secrets))
	for version, secret := range secrets {
		if version <= 0 {
			return nil, fmt.Errorf("token hash secret version must be greater than 0")
		}
		if secret == "" {
			return nil, fmt.Errorf("token hash secret version %d is empty", version)
		}
		hashers[version] = NewTokenHasher(secret)
	}
	if _, ok := hashers[activeVersion]; !ok {
		return nil, fmt.Errorf("active token hash secret version %d is not present in keyring", activeVersion)
	}
	return &TokenHashKeyRing{activeVersion: activeVersion, hashers: hashers}, nil
}

func (r *TokenHashKeyRing) Hash(token string) string {
	return r.hashers[r.activeVersion].Hash(token)
}

func (r *TokenHashKeyRing) CandidateHashes(token string) []string {
	hashes := make([]string, 0, len(r.hashers))
	for _, hasher := range r.hashers {
		hashes = append(hashes, hasher.Hash(token))
	}
	return hashes
}

func (r *TokenHashKeyRing) Matches(token string, storedHash string) bool {
	for _, candidate := range r.CandidateHashes(token) {
		if hmac.Equal([]byte(candidate), []byte(storedHash)) {
			return true
		}
	}
	return false
}
