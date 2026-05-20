package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsesDefaultsAndEnvironment(t *testing.T) {
	t.Setenv("PLATFORM_BOOTSTRAP_TOKEN", "bootstrap-token")
	t.Setenv("TOKEN_HASH_SECRET", "token-hash-secret")
	t.Setenv("SECRET_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	t.Setenv("SERVER_PORT", "9090")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.AppEnv != "development" {
		t.Fatalf("AppEnv = %q, want development", cfg.AppEnv)
	}
	if cfg.ServerPort != 9090 {
		t.Fatalf("ServerPort = %d, want 9090", cfg.ServerPort)
	}
	if cfg.DatabaseURL == "" {
		t.Fatal("DatabaseURL should have a default")
	}
	if cfg.RedisURL == "" {
		t.Fatal("RedisURL should have a default")
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.MaxRetries != 1 {
		t.Fatalf("MaxRetries = %d, want 1", cfg.MaxRetries)
	}
	if cfg.RetryBackoffMS != 100 {
		t.Fatalf("RetryBackoffMS = %d, want 100", cfg.RetryBackoffMS)
	}
}

func TestLoadReadsConfigFileAndEnvironmentOverrides(t *testing.T) {
	t.Setenv("SERVER_PORT", "9091")
	t.Setenv("MAX_RETRIES", "3")
	t.Setenv("RETRY_BACKOFF_MS", "250")

	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
app_env: test
server_port: 8088
database_url: postgres://file
redis_url: redis://file
platform_bootstrap_token: file-bootstrap
token_hash_secret: file-hash-secret
secret_encryption_key: 0123456789abcdef0123456789abcdef
log_level: debug
max_retries: 2
retry_backoff_ms: 125
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if cfg.AppEnv != "test" {
		t.Fatalf("AppEnv = %q, want test", cfg.AppEnv)
	}
	if cfg.ServerPort != 9091 {
		t.Fatalf("ServerPort = %d, want environment override 9091", cfg.ServerPort)
	}
	if cfg.DatabaseURL != "postgres://file" {
		t.Fatalf("DatabaseURL = %q, want postgres://file", cfg.DatabaseURL)
	}
	if cfg.LogLevel != "debug" {
		t.Fatalf("LogLevel = %q, want debug", cfg.LogLevel)
	}
	if cfg.MaxRetries != 3 {
		t.Fatalf("MaxRetries = %d, want environment override 3", cfg.MaxRetries)
	}
	if cfg.RetryBackoffMS != 250 {
		t.Fatalf("RetryBackoffMS = %d, want environment override 250", cfg.RetryBackoffMS)
	}
}

func TestLoadRequiresSecrets(t *testing.T) {
	t.Setenv("PLATFORM_BOOTSTRAP_TOKEN", "")
	t.Setenv("TOKEN_HASH_SECRET", "")
	t.Setenv("SECRET_ENCRYPTION_KEY", "")

	_, err := Load("")
	if err == nil {
		t.Fatal("Load returned nil error, want missing required config error")
	}
}

func TestLoadRejectsInvalidEncryptionKeyLength(t *testing.T) {
	t.Setenv("PLATFORM_BOOTSTRAP_TOKEN", "bootstrap-token")
	t.Setenv("TOKEN_HASH_SECRET", "token-hash-secret")
	t.Setenv("SECRET_ENCRYPTION_KEY", "too-short")

	_, err := Load("")
	if err == nil {
		t.Fatal("Load returned nil error, want invalid encryption key length")
	}
}

func TestLoadRejectsInvalidBoundaryValues(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "invalid app env",
			env: map[string]string{
				"APP_ENV": "stagingish",
			},
		},
		{
			name: "invalid database url",
			env: map[string]string{
				"DATABASE_URL": "not a url",
			},
		},
		{
			name: "invalid redis url",
			env: map[string]string{
				"REDIS_URL": "postgres://localhost:5432/db",
			},
		},
		{
			name: "invalid log level",
			env: map[string]string{
				"LOG_LEVEL": "verbose",
			},
		},
		{
			name: "server port is not a number",
			env: map[string]string{
				"SERVER_PORT": "http",
			},
		},
		{
			name: "server port is too high",
			env: map[string]string{
				"SERVER_PORT": "70000",
			},
		},
		{
			name: "max retries is not a number",
			env: map[string]string{
				"MAX_RETRIES": "many",
			},
		},
		{
			name: "max retries is negative",
			env: map[string]string{
				"MAX_RETRIES": "-1",
			},
		},
		{
			name: "retry backoff is not a number",
			env: map[string]string{
				"RETRY_BACKOFF_MS": "slow",
			},
		},
		{
			name: "retry backoff is zero",
			env: map[string]string{
				"RETRY_BACKOFF_MS": "0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("PLATFORM_BOOTSTRAP_TOKEN", "bootstrap-token")
			t.Setenv("TOKEN_HASH_SECRET", "token-hash-secret")
			t.Setenv("SECRET_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")

			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			if _, err := Load(""); err == nil {
				t.Fatal("Load returned nil error, want validation error")
			}
		})
	}
}

func TestLoadReturnsErrorForInvalidConfigFile(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
			t.Fatal("Load returned nil error, want missing file error")
		}
	})

	t.Run("invalid yaml", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		if err := os.WriteFile(path, []byte("app_env: ["), 0o600); err != nil {
			t.Fatalf("write config file: %v", err)
		}

		if _, err := Load(path); err == nil {
			t.Fatal("Load returned nil error, want invalid YAML error")
		}
	})
}
