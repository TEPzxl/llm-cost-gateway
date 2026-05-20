package app

import (
	"github.com/gin-gonic/gin"
	"github.com/tep/llm-cost-gateway/internal/http/handlers/health"
)

func NewRouter(appEnv string) *gin.Engine {
	gin.SetMode(ginMode(appEnv))

	router := gin.New()
	router.Use(gin.Recovery())
	router.GET("/healthz", health.Health)
	return router
}

func ginMode(appEnv string) string {
	switch appEnv {
	case "production":
		return gin.ReleaseMode
	case "test":
		return gin.TestMode
	default:
		return gin.DebugMode
	}
}
