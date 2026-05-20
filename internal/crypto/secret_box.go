package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

const aes256KeyLength = 32

type SecretBox struct {
	gcm cipher.AEAD
}

type SealedSecret struct {
	Encrypted string
	Nonce     string
}

func NewSecretBox(key string) (*SecretBox, error) {
	if len(key) != aes256KeyLength {
		return nil, fmt.Errorf("secret encryption key must be %d bytes", aes256KeyLength)
	}

	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretBox{gcm: gcm}, nil
}

func (b *SecretBox) Seal(plaintext string) (SealedSecret, error) {
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return SealedSecret{}, err
	}

	ciphertext := b.gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return SealedSecret{
		Encrypted: base64.StdEncoding.EncodeToString(ciphertext),
		Nonce:     base64.StdEncoding.EncodeToString(nonce),
	}, nil
}

func (b *SecretBox) Open(encrypted string, nonce string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}
	nonceBytes, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		return "", err
	}

	plaintext, err := b.gcm.Open(nil, nonceBytes, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
