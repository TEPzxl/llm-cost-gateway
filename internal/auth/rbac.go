package auth

const (
	ActorServiceToken = "service_token"
	ActorUser         = "user"

	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
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

func (p AdminTokenPrincipal) CanManageMembers() bool {
	if p.IsServiceToken() {
		return true
	}
	return p.Role == RoleOwner
}

func (p AdminTokenPrincipal) CanManageTokens() bool {
	if p.IsServiceToken() {
		return true
	}
	return p.Role == RoleOwner
}

func (p AdminTokenPrincipal) CanManageConfiguration() bool {
	if p.IsServiceToken() {
		return true
	}
	return p.Role == RoleOwner || p.Role == RoleAdmin
}

func (p AdminTokenPrincipal) CanView() bool {
	if p.IsServiceToken() {
		return true
	}
	return p.Role == RoleOwner || p.Role == RoleAdmin || p.Role == RoleViewer
}
