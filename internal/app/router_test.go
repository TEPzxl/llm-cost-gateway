package app

import (
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func TestNewSetsGinModeFromEnvironment(t *testing.T) {
	t.Cleanup(func() {
		gin.SetMode(gin.DebugMode)
	})

	tests := []struct {
		name string
		env  string
		want string
	}{
		{name: "production uses release mode", env: "production", want: gin.ReleaseMode},
		{name: "test uses test mode", env: "test", want: gin.TestMode},
		{name: "development uses debug mode", env: "development", want: gin.DebugMode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.DebugMode)

			_ = NewRouter(RouterConfig{
				AppEnv: tt.env,
			}, zap.NewNop())

			if got := gin.Mode(); got != tt.want {
				t.Fatalf("gin mode = %q, want %q", got, tt.want)
			}
		})
	}
}
