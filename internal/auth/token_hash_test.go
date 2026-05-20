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
