package crypto

import "fmt"

type SecretKeyRing struct {
	activeVersion int32
	boxes         map[int32]*SecretBox
}

func NewSingleKeyRing(key string) (*SecretKeyRing, error) {
	return NewSecretKeyRing(1, map[int32]string{1: key})
}

func NewSecretKeyRing(activeVersion int32, keys map[int32]string) (*SecretKeyRing, error) {
	if activeVersion <= 0 {
		return nil, fmt.Errorf("active key version must be greater than 0")
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("secret keyring must contain at least one key")
	}
	boxes := make(map[int32]*SecretBox, len(keys))
	for version, key := range keys {
		if version <= 0 {
			return nil, fmt.Errorf("key version must be greater than 0")
		}
		box, err := NewSecretBox(key)
		if err != nil {
			return nil, fmt.Errorf("key version %d: %w", version, err)
		}
		boxes[version] = box
	}
	if _, ok := boxes[activeVersion]; !ok {
		return nil, fmt.Errorf("active key version %d is not present in keyring", activeVersion)
	}
	return &SecretKeyRing{activeVersion: activeVersion, boxes: boxes}, nil
}

func (r *SecretKeyRing) ActiveVersion() int32 {
	return r.activeVersion
}

func (r *SecretKeyRing) Seal(plaintext string) (SealedSecret, error) {
	box := r.boxes[r.activeVersion]
	sealed, err := box.Seal(plaintext)
	if err != nil {
		return SealedSecret{}, err
	}
	sealed.KeyVersion = r.activeVersion
	return sealed, nil
}

func (r *SecretKeyRing) Open(encrypted string, nonce string, keyVersion int32) (string, error) {
	box, ok := r.boxes[keyVersion]
	if !ok {
		return "", fmt.Errorf("key version %d is not present in keyring", keyVersion)
	}
	return box.Open(encrypted, nonce)
}
