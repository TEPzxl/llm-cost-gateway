package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/audit"
)

const (
	adminAuditResourceTypeKey = "admin_audit_resource_type"
	adminAuditResourceIDKey   = "admin_audit_resource_id"
	adminAuditActionKey       = "admin_audit_action"
)

func SetAdminAuditResource(c *gin.Context, resourceType string, resourceID uuid.UUID) {
	c.Set(adminAuditResourceTypeKey, resourceType)
	c.Set(adminAuditResourceIDKey, resourceID)
}

func SetAdminAuditAction(c *gin.Context, action string) {
	c.Set(adminAuditActionKey, action)
}

func AdminAudit(service *audit.AdminAuditService) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if service == nil || !isAdminMutation(c.Request.Method) || c.Writer.Status() >= http.StatusBadRequest {
			return
		}
		principal, ok := AdminTokenPrincipalFromContext(c)
		if !ok {
			return
		}
		resourceType, ok := auditStringValue(c, adminAuditResourceTypeKey)
		if !ok {
			return
		}
		action, ok := auditStringValue(c, adminAuditActionKey)
		if !ok {
			action = defaultAuditAction(c.Request.Method, resourceType)
		}
		resourceID := auditUUIDPtr(c, adminAuditResourceIDKey)
		var adminTokenID *uuid.UUID
		var userID *uuid.UUID
		if principal.IsUser() && principal.UserID != nil {
			userID = principal.UserID
		} else {
			tokenID := principal.AdminTokenID
			adminTokenID = &tokenID
		}
		_, _ = service.Record(c.Request.Context(), audit.RecordInput{
			OrgID:             principal.OrgID,
			ActorAdminTokenID: adminTokenID,
			ActorUserID:       userID,
			Action:            action,
			ResourceType:      resourceType,
			ResourceID:        resourceID,
			RequestID:         RequestIDFromContext(c),
		})
	}
}

func isAdminMutation(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

func defaultAuditAction(method string, resourceType string) string {
	switch method {
	case http.MethodPost:
		return "create_" + resourceType
	case http.MethodPatch, http.MethodPut:
		return "update_" + resourceType
	case http.MethodDelete:
		return "delete_" + resourceType
	default:
		return resourceType
	}
}

func auditStringValue(c *gin.Context, key string) (string, bool) {
	value, ok := c.Get(key)
	if !ok {
		return "", false
	}
	result, ok := value.(string)
	return result, ok && result != ""
}

func auditUUIDPtr(c *gin.Context, key string) *uuid.UUID {
	value, ok := c.Get(key)
	if !ok {
		return nil
	}
	result, ok := value.(uuid.UUID)
	if !ok {
		return nil
	}
	return &result
}
