package auth

import "strings"

const (
	ActorServiceToken = "service_token"
	ActorUser         = "user"

	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleViewer = "viewer"

	AdminScopeAll                 = "admin:*"
	AdminScopeMembersManage       = "admin:members:manage"
	AdminScopeMembersView         = "admin:members:view"
	AdminScopeTokensManage        = "admin:tokens:manage"
	AdminScopeConfigurationManage = "admin:configuration:manage"
	AdminScopeView                = "admin:view"

	APIKeyScopeChatCompletions = "chat.completions"
)

func ValidRole(role string) bool {
	switch role {
	case RoleOwner, RoleAdmin, RoleViewer:
		return true
	default:
		return false
	}
}

func (p AdminTokenPrincipal) IsServiceToken() bool {
	return p.ActorType == "" || p.ActorType == ActorServiceToken
}

func (p AdminTokenPrincipal) IsUser() bool {
	return p.ActorType == ActorUser
}

func (p AdminTokenPrincipal) HasScope(scope string) bool {
	return scopesAllow(p.Scopes, scope)
}

func (p AdminTokenPrincipal) hasAnyScope(scopes ...string) bool {
	for _, scope := range scopes {
		if p.HasScope(scope) {
			return true
		}
	}
	return false
}

func (p AdminTokenPrincipal) CanManageMembers() bool {
	if p.IsServiceToken() {
		return p.HasScope(AdminScopeMembersManage)
	}
	return p.Role == RoleOwner
}

func (p AdminTokenPrincipal) CanViewMembers() bool {
	if p.IsServiceToken() {
		return p.hasAnyScope(AdminScopeMembersView, AdminScopeMembersManage)
	}
	return p.Role == RoleOwner || p.Role == RoleAdmin
}

func (p AdminTokenPrincipal) CanManageTokens() bool {
	if p.IsServiceToken() {
		return p.HasScope(AdminScopeTokensManage)
	}
	return p.Role == RoleOwner
}

func (p AdminTokenPrincipal) CanManageConfiguration() bool {
	if p.IsServiceToken() {
		return p.HasScope(AdminScopeConfigurationManage)
	}
	return p.Role == RoleOwner || p.Role == RoleAdmin
}

func (p AdminTokenPrincipal) CanView() bool {
	if p.IsServiceToken() {
		return p.HasScope(AdminScopeView)
	}
	return p.Role == RoleOwner || p.Role == RoleAdmin || p.Role == RoleViewer
}

func (p APIKeyPrincipal) HasScope(scope string) bool {
	required := strings.TrimSpace(scope)
	if required == "" {
		return false
	}
	for _, item := range p.Scopes {
		if strings.TrimSpace(item) == required {
			return true
		}
	}
	return false
}

func scopesAllow(scopes []string, required string) bool {
	required = strings.TrimSpace(required)
	if required == "" {
		return false
	}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		switch {
		case scope == "":
			continue
		case scope == "*" || scope == required:
			return true
		case strings.HasSuffix(scope, "*"):
			prefix := strings.TrimSuffix(scope, "*")
			if prefix != "" && strings.HasPrefix(required, prefix) {
				return true
			}
		}
	}
	return false
}
