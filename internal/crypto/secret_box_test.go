package crypto

import "testing"

func TestSecretBoxEncryptsAndDecryptsPlaintext(t *testing.T) {
	box, err := NewSecretBox("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretBox returned error: %v", err)
	}

	sealed, err := box.Seal("provider-secret-key")
	if err != nil {
		t.Fatalf("Seal returned error: %v", err)
	}
	if sealed.Encrypted == "" {
		t.Fatal("sealed encrypted value is empty")
	}
	if sealed.Nonce == "" {
		t.Fatal("sealed nonce is empty")
	}
	if sealed.Encrypted == "provider-secret-key" {
		t.Fatal("encrypted value equals plaintext")
	}

	got, err := box.Open(sealed.Encrypted, sealed.Nonce)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	if got != "provider-secret-key" {
		t.Fatalf("decrypted value = %q, want provider-secret-key", got)
	}
}

func TestSecretBoxRejectsInvalidKeyLength(t *testing.T) {
	if _, err := NewSecretBox("short-key"); err == nil {
		t.Fatal("NewSecretBox returned nil error for invalid key length")
	}
}
