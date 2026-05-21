package cache

import (
	"encoding/json"
	"fmt"

	secretcrypto "github.com/tep/llm-cost-gateway/internal/crypto"
)

const encryptedCachedChatResponseVersion = 1

type encryptedCachedChatResponse struct {
	Version   int    `json:"version"`
	Encrypted string `json:"encrypted"`
	Nonce     string `json:"nonce"`
}

type responseCodec struct {
	box *secretcrypto.SecretBox
}

func newResponseCodec(secretEncryptionKey string) (responseCodec, error) {
	box, err := secretcrypto.NewSecretBox(secretEncryptionKey)
	if err != nil {
		return responseCodec{}, err
	}
	return responseCodec{box: box}, nil
}

func (c responseCodec) Seal(response CachedChatResponse) (encryptedCachedChatResponse, error) {
	if c.box == nil {
		return encryptedCachedChatResponse{}, fmt.Errorf("cache response encryption is not configured")
	}
	body, err := json.Marshal(response)
	if err != nil {
		return encryptedCachedChatResponse{}, err
	}
	sealed, err := c.box.Seal(string(body))
	if err != nil {
		return encryptedCachedChatResponse{}, err
	}
	return encryptedCachedChatResponse{
		Version:   encryptedCachedChatResponseVersion,
		Encrypted: sealed.Encrypted,
		Nonce:     sealed.Nonce,
	}, nil
}

func (c responseCodec) Open(sealed encryptedCachedChatResponse) (CachedChatResponse, error) {
	if c.box == nil {
		return CachedChatResponse{}, fmt.Errorf("cache response encryption is not configured")
	}
	if sealed.Version != encryptedCachedChatResponseVersion || sealed.Encrypted == "" || sealed.Nonce == "" {
		return CachedChatResponse{}, fmt.Errorf("invalid encrypted cached response")
	}
	plaintext, err := c.box.Open(sealed.Encrypted, sealed.Nonce)
	if err != nil {
		return CachedChatResponse{}, err
	}
	var response CachedChatResponse
	if err := json.Unmarshal([]byte(plaintext), &response); err != nil {
		return CachedChatResponse{}, err
	}
	return response, nil
}
