package crypto

import "testing"

func TestSecretKeyRingSealsWithActiveVersionAndOpensOldVersion(t *testing.T) {
	ring, err := NewSecretKeyRing(2, map[int32]string{
		1: "0123456789abcdef0123456789abcdef",
		2: "fedcba98765432100123456789abcdef",
	})
	if err != nil {
		t.Fatalf("NewSecretKeyRing returned error: %v", err)
	}

	oldBox, err := NewSecretBox("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("NewSecretBox returned error: %v", err)
	}
	oldSealed, err := oldBox.Seal("old-provider-key")
	if err != nil {
		t.Fatalf("Seal old secret returned error: %v", err)
	}

	openedOld, err := ring.Open(oldSealed.Encrypted, oldSealed.Nonce, 1)
	if err != nil {
		t.Fatalf("Open old version returned error: %v", err)
	}
	if openedOld != "old-provider-key" {
		t.Fatalf("opened old secret = %q, want old-provider-key", openedOld)
	}

	newSealed, err := ring.Seal("new-provider-key")
	if err != nil {
		t.Fatalf("Seal returned error: %v", err)
	}
	if newSealed.KeyVersion != 2 {
		t.Fatalf("sealed key version = %d, want 2", newSealed.KeyVersion)
	}
	openedNew, err := ring.Open(newSealed.Encrypted, newSealed.Nonce, newSealed.KeyVersion)
	if err != nil {
		t.Fatalf("Open new version returned error: %v", err)
	}
	if openedNew != "new-provider-key" {
		t.Fatalf("opened new secret = %q, want new-provider-key", openedNew)
	}
}

func TestSecretKeyRingRejectsMissingActiveVersion(t *testing.T) {
	if _, err := NewSecretKeyRing(2, map[int32]string{
		1: "0123456789abcdef0123456789abcdef",
	}); err == nil {
		t.Fatal("NewSecretKeyRing returned nil error, want missing active version error")
	}
}
