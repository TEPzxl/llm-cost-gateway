package auth

import "testing"

func TestTokenHasherHashesWithSecret(t *testing.T) {
	token := "llmgw_admin_test-token"

	hasher := NewTokenHasher("secret-a")
	got := hasher.Hash(token)

	if got == "" {
		t.Fatal("Hash returned empty string")
	}
	if got == token {
		t.Fatal("Hash returned plaintext token")
	}
	if got != hasher.Hash(token) {
		t.Fatal("Hash returned different values for the same token and secret")
	}
	if got == NewTokenHasher("secret-b").Hash(token) {
		t.Fatal("Hash returned the same value for different secrets")
	}
}

func TestTokenHashKeyRingUsesActiveSecretAndMatchesOldSecrets(t *testing.T) {
	ring, err := NewTokenHashKeyRing(2, map[int32]string{
		1: "old-token-hash-secret",
		2: "new-token-hash-secret",
	})
	if err != nil {
		t.Fatalf("NewTokenHashKeyRing returned error: %v", err)
	}
	token := "llmgw_admin_test-token"

	activeHash := ring.Hash(token)
	if activeHash != NewTokenHasher("new-token-hash-secret").Hash(token) {
		t.Fatal("active hash did not use active secret")
	}
	oldHash := NewTokenHasher("old-token-hash-secret").Hash(token)
	if !ring.Matches(token, oldHash) {
		t.Fatal("keyring did not match hash generated with old secret")
	}
	if ring.Matches(token, "not-a-real-hash") {
		t.Fatal("keyring matched unrelated hash")
	}
}
