package auth

import "testing"

func TestAdminServiceTokenScopes(t *testing.T) {
	tests := []struct {
		name      string
		scopes    []string
		operation func(AdminTokenPrincipal) bool
		want      bool
	}{
		{name: "admin wildcard can manage tokens", scopes: []string{AdminScopeAll}, operation: AdminTokenPrincipal.CanManageTokens, want: true},
		{name: "specific token scope can manage tokens", scopes: []string{AdminScopeTokensManage}, operation: AdminTokenPrincipal.CanManageTokens, want: true},
		{name: "view scope cannot manage tokens", scopes: []string{AdminScopeView}, operation: AdminTokenPrincipal.CanManageTokens, want: false},
		{name: "member manage scope can view members", scopes: []string{AdminScopeMembersManage}, operation: AdminTokenPrincipal.CanViewMembers, want: true},
		{name: "empty scopes cannot manage configuration", scopes: nil, operation: AdminTokenPrincipal.CanManageConfiguration, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			principal := AdminTokenPrincipal{ActorType: ActorServiceToken, Scopes: tt.scopes}
			if got := tt.operation(principal); got != tt.want {
				t.Fatalf("operation = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUserRolesStillUseRBAC(t *testing.T) {
	viewer := AdminTokenPrincipal{ActorType: ActorUser, Role: RoleViewer}
	if !viewer.CanView() {
		t.Fatal("viewer CanView = false, want true")
	}
	if viewer.CanManageConfiguration() {
		t.Fatal("viewer CanManageConfiguration = true, want false")
	}

	owner := AdminTokenPrincipal{ActorType: ActorUser, Role: RoleOwner}
	if !owner.CanManageMembers() || !owner.CanManageTokens() {
		t.Fatal("owner should manage members and tokens")
	}
}

func TestAPIKeyPrincipalScopes(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		want   bool
	}{
		{name: "exact scope", scopes: []string{APIKeyScopeChatCompletions}, want: true},
		{name: "wildcard scope is not accepted for api keys", scopes: []string{"chat.*"}, want: false},
		{name: "global wildcard is not accepted for api keys", scopes: []string{"*"}, want: false},
		{name: "wrong separator", scopes: []string{"chat:completions"}, want: false},
		{name: "empty scopes", scopes: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			principal := APIKeyPrincipal{Scopes: tt.scopes}
			if got := principal.HasScope(APIKeyScopeChatCompletions); got != tt.want {
				t.Fatalf("HasScope = %v, want %v", got, tt.want)
			}
		})
	}
}
