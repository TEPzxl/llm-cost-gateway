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
	t.Setenv("SECRET_ENCRYPTION_KEY_VERSION", "2")
	t.Setenv("SECRET_ENCRYPTION_KEYRING", "1:0123456789abcdef0123456789abcdef,2:fedcba98765432100123456789abcdef")
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
	if cfg.SecretEncryptionKeyVersion != 2 {
		t.Fatalf("SecretEncryptionKeyVersion = %d, want 2", cfg.SecretEncryptionKeyVersion)
	}
	keys, err := cfg.SecretEncryptionKeys()
	if err != nil {
		t.Fatalf("SecretEncryptionKeys returned error: %v", err)
	}
	if len(keys) != 2 || keys[2] != "fedcba98765432100123456789abcdef" {
		t.Fatalf("SecretEncryptionKeys = %v, want versions 1 and 2", keys)
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
	if cfg.KafkaEnabled {
		t.Fatal("KafkaEnabled = true, want false")
	}
	if len(cfg.KafkaBrokers) != 0 {
		t.Fatalf("KafkaBrokers = %v, want empty", cfg.KafkaBrokers)
	}
	if cfg.KafkaUsageTopic != "llm-usage-events" {
		t.Fatalf("KafkaUsageTopic = %q, want llm-usage-events", cfg.KafkaUsageTopic)
	}
	if cfg.ClickHouseEnabled {
		t.Fatal("ClickHouseEnabled = true, want false")
	}
	if cfg.ClickHouseURL != "http://localhost:8123" {
		t.Fatalf("ClickHouseURL = %q, want http://localhost:8123", cfg.ClickHouseURL)
	}
	if cfg.ClickHouseDatabase != "llmgw" {
		t.Fatalf("ClickHouseDatabase = %q, want llmgw", cfg.ClickHouseDatabase)
	}
	if cfg.ClickHouseUsername != "default" {
		t.Fatalf("ClickHouseUsername = %q, want default", cfg.ClickHouseUsername)
	}
	if !cfg.PromptCacheEnabled {
		t.Fatal("PromptCacheEnabled = false, want true")
	}
	if cfg.PromptCacheTTLSeconds != 300 {
		t.Fatalf("PromptCacheTTLSeconds = %d, want 300", cfg.PromptCacheTTLSeconds)
	}
	if cfg.SemanticCacheEnabled {
		t.Fatal("SemanticCacheEnabled = true, want false")
	}
	if cfg.SemanticCacheThreshold != 0.92 {
		t.Fatalf("SemanticCacheThreshold = %f, want 0.92", cfg.SemanticCacheThreshold)
	}
	if cfg.SemanticCacheMaxTemp != 0.3 {
		t.Fatalf("SemanticCacheMaxTemp = %f, want 0.3", cfg.SemanticCacheMaxTemp)
	}
	if cfg.TracingEnabled {
		t.Fatal("TracingEnabled = true, want false")
	}
	if cfg.TracingOTLPEndpoint != "localhost:4318" {
		t.Fatalf("TracingOTLPEndpoint = %q, want localhost:4318", cfg.TracingOTLPEndpoint)
	}
	if !cfg.TracingInsecure {
		t.Fatal("TracingInsecure = false, want true")
	}
	if cfg.TracingServiceName != "llm-cost-gateway" {
		t.Fatalf("TracingServiceName = %q, want llm-cost-gateway", cfg.TracingServiceName)
	}
	if !cfg.OutboundPublicOnly {
		t.Fatal("OutboundPublicOnly = false, want true by default")
	}
	if cfg.PasswordlessEmailEnabled {
		t.Fatal("PasswordlessEmailEnabled = true, want false")
	}
	if cfg.MagicLinkTTLSeconds != 900 {
		t.Fatalf("MagicLinkTTLSeconds = %d, want 900", cfg.MagicLinkTTLSeconds)
	}
}

func TestLoadReadsConfigFileAndEnvironmentOverrides(t *testing.T) {
	t.Setenv("SERVER_PORT", "9091")
	t.Setenv("MAX_RETRIES", "3")
	t.Setenv("RETRY_BACKOFF_MS", "250")
	t.Setenv("KAFKA_ENABLED", "true")
	t.Setenv("KAFKA_BROKERS", " kafka-1:9092, kafka-2:9092 ")
	t.Setenv("KAFKA_USAGE_TOPIC", "env-usage-events")
	t.Setenv("CLICKHOUSE_ENABLED", "true")
	t.Setenv("CLICKHOUSE_URL", "https://clickhouse.example.test:8123")
	t.Setenv("CLICKHOUSE_DATABASE", "env_llmgw")
	t.Setenv("CLICKHOUSE_USERNAME", "env_user")
	t.Setenv("CLICKHOUSE_PASSWORD", "env_password")
	t.Setenv("PROMPT_CACHE_ENABLED", "false")
	t.Setenv("PROMPT_CACHE_TTL_SECONDS", "60")
	t.Setenv("SEMANTIC_CACHE_ENABLED", "true")
	t.Setenv("SEMANTIC_CACHE_THRESHOLD", "0.8")
	t.Setenv("SEMANTIC_CACHE_MAX_TEMPERATURE", "0.2")
	t.Setenv("TRACING_ENABLED", "true")
	t.Setenv("TRACING_OTLP_ENDPOINT", "https://otel.example.test:4318")
	t.Setenv("TRACING_INSECURE", "false")
	t.Setenv("TRACING_SERVICE_NAME", "env-gateway")
	t.Setenv("OUTBOUND_PUBLIC_ONLY", "false")
	t.Setenv("SECRET_ENCRYPTION_KEY_VERSION", "2")
	t.Setenv("SECRET_ENCRYPTION_KEYRING", "1:0123456789abcdef0123456789abcdef,2:fedcba98765432100123456789abcdef")
	t.Setenv("AUTH_PASSWORDLESS_EMAIL_ENABLED", "true")
	t.Setenv("AUTH_MAGIC_LINK_BASE_URL", "https://console.example.test/login")
	t.Setenv("AUTH_MAGIC_LINK_TTL_SECONDS", "600")
	t.Setenv("SMTP_HOST", "smtp.example.test")
	t.Setenv("SMTP_PORT", "2525")
	t.Setenv("SMTP_USERNAME", "env-smtp-user")
	t.Setenv("SMTP_PASSWORD", "env-smtp-password")
	t.Setenv("SMTP_FROM", "login@example.test")
	t.Setenv("SMTP_TLS_MODE", "implicit")

	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
app_env: test
server_port: 8088
database_url: postgres://file
redis_url: redis://file
platform_bootstrap_token: file-bootstrap
token_hash_secret: file-hash-secret
secret_encryption_key: 0123456789abcdef0123456789abcdef
secret_encryption_key_version: 1
secret_encryption_keyring: "1:0123456789abcdef0123456789abcdef"
log_level: debug
max_retries: 2
retry_backoff_ms: 125
kafka_enabled: false
kafka_brokers:
  - file-kafka:9092
kafka_usage_topic: file-usage-events
clickhouse_enabled: false
clickhouse_url: http://file-clickhouse:8123
clickhouse_database: file_llmgw
clickhouse_username: file_user
clickhouse_password: file_password
prompt_cache_enabled: true
prompt_cache_ttl_seconds: 120
semantic_cache_enabled: false
semantic_cache_threshold: 0.9
semantic_cache_max_temperature: 0.4
tracing_enabled: false
tracing_otlp_endpoint: localhost:4318
tracing_insecure: true
tracing_service_name: file-gateway
outbound_public_only: true
auth_passwordless_email_enabled: false
auth_magic_link_base_url: https://file-console.example.test/login
auth_magic_link_ttl_seconds: 900
smtp_host: file-smtp.example.test
smtp_port: 587
smtp_username: file_user
smtp_password: file_password
smtp_from: file-login@example.test
smtp_tls_mode: starttls
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
	if cfg.SecretEncryptionKeyVersion != 2 {
		t.Fatalf("SecretEncryptionKeyVersion = %d, want environment override 2", cfg.SecretEncryptionKeyVersion)
	}
	if cfg.MaxRetries != 3 {
		t.Fatalf("MaxRetries = %d, want environment override 3", cfg.MaxRetries)
	}
	if cfg.RetryBackoffMS != 250 {
		t.Fatalf("RetryBackoffMS = %d, want environment override 250", cfg.RetryBackoffMS)
	}
	if !cfg.KafkaEnabled {
		t.Fatal("KafkaEnabled = false, want environment override true")
	}
	wantBrokers := []string{"kafka-1:9092", "kafka-2:9092"}
	if len(cfg.KafkaBrokers) != len(wantBrokers) {
		t.Fatalf("KafkaBrokers = %v, want %v", cfg.KafkaBrokers, wantBrokers)
	}
	for index, want := range wantBrokers {
		if cfg.KafkaBrokers[index] != want {
			t.Fatalf("KafkaBrokers[%d] = %q, want %q", index, cfg.KafkaBrokers[index], want)
		}
	}
	if cfg.KafkaUsageTopic != "env-usage-events" {
		t.Fatalf("KafkaUsageTopic = %q, want env-usage-events", cfg.KafkaUsageTopic)
	}
	if !cfg.ClickHouseEnabled {
		t.Fatal("ClickHouseEnabled = false, want environment override true")
	}
	if cfg.ClickHouseURL != "https://clickhouse.example.test:8123" {
		t.Fatalf("ClickHouseURL = %q, want env URL", cfg.ClickHouseURL)
	}
	if cfg.ClickHouseDatabase != "env_llmgw" {
		t.Fatalf("ClickHouseDatabase = %q, want env_llmgw", cfg.ClickHouseDatabase)
	}
	if cfg.ClickHouseUsername != "env_user" {
		t.Fatalf("ClickHouseUsername = %q, want env_user", cfg.ClickHouseUsername)
	}
	if cfg.ClickHousePassword != "env_password" {
		t.Fatalf("ClickHousePassword = %q, want env_password", cfg.ClickHousePassword)
	}
	if cfg.PromptCacheEnabled {
		t.Fatal("PromptCacheEnabled = true, want environment override false")
	}
	if cfg.PromptCacheTTLSeconds != 60 {
		t.Fatalf("PromptCacheTTLSeconds = %d, want environment override 60", cfg.PromptCacheTTLSeconds)
	}
	if !cfg.SemanticCacheEnabled {
		t.Fatal("SemanticCacheEnabled = false, want environment override true")
	}
	if cfg.SemanticCacheThreshold != 0.8 {
		t.Fatalf("SemanticCacheThreshold = %f, want 0.8", cfg.SemanticCacheThreshold)
	}
	if cfg.SemanticCacheMaxTemp != 0.2 {
		t.Fatalf("SemanticCacheMaxTemp = %f, want 0.2", cfg.SemanticCacheMaxTemp)
	}
	if !cfg.TracingEnabled {
		t.Fatal("TracingEnabled = false, want environment override true")
	}
	if cfg.TracingOTLPEndpoint != "https://otel.example.test:4318" {
		t.Fatalf("TracingOTLPEndpoint = %q, want env endpoint", cfg.TracingOTLPEndpoint)
	}
	if cfg.TracingInsecure {
		t.Fatal("TracingInsecure = true, want environment override false")
	}
	if cfg.TracingServiceName != "env-gateway" {
		t.Fatalf("TracingServiceName = %q, want env-gateway", cfg.TracingServiceName)
	}
	if cfg.OutboundPublicOnly {
		t.Fatal("OutboundPublicOnly = true, want environment override false")
	}
	if !cfg.PasswordlessEmailEnabled {
		t.Fatal("PasswordlessEmailEnabled = false, want environment override true")
	}
	if cfg.MagicLinkBaseURL != "https://console.example.test/login" {
		t.Fatalf("MagicLinkBaseURL = %q, want env URL", cfg.MagicLinkBaseURL)
	}
	if cfg.MagicLinkTTLSeconds != 600 {
		t.Fatalf("MagicLinkTTLSeconds = %d, want environment override 600", cfg.MagicLinkTTLSeconds)
	}
	if cfg.SMTPHost != "smtp.example.test" || cfg.SMTPPort != 2525 || cfg.SMTPUsername != "env-smtp-user" || cfg.SMTPPassword != "env-smtp-password" || cfg.SMTPFrom != "login@example.test" || cfg.SMTPTLSMode != "implicit" {
		t.Fatal("SMTP config did not use environment overrides")
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

func TestLoadRejectsUnsafeProductionConfig(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "weak platform token",
			env: map[string]string{
				"PLATFORM_BOOTSTRAP_TOKEN": "dev-bootstrap-token",
			},
		},
		{
			name: "weak token hash secret",
			env: map[string]string{
				"TOKEN_HASH_SECRET": "dev-token-hash-secret",
			},
		},
		{
			name: "example encryption key",
			env: map[string]string{
				"SECRET_ENCRYPTION_KEY": "0123456789abcdef0123456789abcdef",
			},
		},
		{
			name: "database ssl disabled",
			env: map[string]string{
				"DATABASE_URL": "postgres://llmgw:llmgw@example.com:5432/llmgw?sslmode=disable",
			},
		},
		{
			name: "public outbound guard disabled",
			env: map[string]string{
				"OUTBOUND_PUBLIC_ONLY": "false",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("APP_ENV", "production")
			t.Setenv("DATABASE_URL", "postgres://llmgw:llmgw@example.com:5432/llmgw?sslmode=require")
			t.Setenv("PLATFORM_BOOTSTRAP_TOKEN", "prod-bootstrap-token-012345678901")
			t.Setenv("TOKEN_HASH_SECRET", "prod-token-hash-secret-0123456789")
			t.Setenv("SECRET_ENCRYPTION_KEY", "fedcba98765432100123456789abcdef")

			for key, value := range tt.env {
				t.Setenv(key, value)
			}

			if _, err := Load(""); err == nil {
				t.Fatal("Load returned nil error, want unsafe production config error")
			}
		})
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
		{
			name: "kafka enabled is not a bool",
			env: map[string]string{
				"KAFKA_ENABLED": "maybe",
			},
		},
		{
			name: "kafka enabled without brokers",
			env: map[string]string{
				"KAFKA_ENABLED": "true",
			},
		},
		{
			name: "clickhouse enabled is not a bool",
			env: map[string]string{
				"CLICKHOUSE_ENABLED": "maybe",
			},
		},
		{
			name: "clickhouse enabled with invalid url",
			env: map[string]string{
				"CLICKHOUSE_ENABLED": "true",
				"CLICKHOUSE_URL":     "postgres://localhost:8123",
			},
		},
		{
			name: "prompt cache enabled is not a bool",
			env: map[string]string{
				"PROMPT_CACHE_ENABLED": "sometimes",
			},
		},
		{
			name: "prompt cache ttl is not a number",
			env: map[string]string{
				"PROMPT_CACHE_TTL_SECONDS": "soon",
			},
		},
		{
			name: "prompt cache ttl is zero",
			env: map[string]string{
				"PROMPT_CACHE_TTL_SECONDS": "0",
			},
		},
		{
			name: "semantic cache enabled is not a bool",
			env: map[string]string{
				"SEMANTIC_CACHE_ENABLED": "sometimes",
			},
		},
		{
			name: "semantic cache threshold is not a number",
			env: map[string]string{
				"SEMANTIC_CACHE_THRESHOLD": "close",
			},
		},
		{
			name: "semantic cache threshold is too high",
			env: map[string]string{
				"SEMANTIC_CACHE_THRESHOLD": "1.1",
			},
		},
		{
			name: "semantic cache max temperature is negative",
			env: map[string]string{
				"SEMANTIC_CACHE_MAX_TEMPERATURE": "-0.1",
			},
		},
		{
			name: "tracing enabled is not a bool",
			env: map[string]string{
				"TRACING_ENABLED": "sometimes",
			},
		},
		{
			name: "tracing insecure is not a bool",
			env: map[string]string{
				"TRACING_INSECURE": "sometimes",
			},
		},
		{
			name: "tracing enabled without endpoint",
			env: map[string]string{
				"TRACING_ENABLED":       "true",
				"TRACING_OTLP_ENDPOINT": "   ",
			},
		},
		{
			name: "secret key version is not a number",
			env: map[string]string{
				"SECRET_ENCRYPTION_KEY_VERSION": "latest",
			},
		},
		{
			name: "secret keyring missing active version",
			env: map[string]string{
				"SECRET_ENCRYPTION_KEY_VERSION": "2",
				"SECRET_ENCRYPTION_KEYRING":     "1:0123456789abcdef0123456789abcdef",
			},
		},
		{
			name: "secret keyring invalid entry",
			env: map[string]string{
				"SECRET_ENCRYPTION_KEYRING": "not-a-version",
			},
		},
		{
			name: "outbound public only is not a bool",
			env: map[string]string{
				"OUTBOUND_PUBLIC_ONLY": "sometimes",
			},
		},
		{
			name: "passwordless email enabled is not a bool",
			env: map[string]string{
				"AUTH_PASSWORDLESS_EMAIL_ENABLED": "sometimes",
			},
		},
		{
			name: "passwordless email enabled without magic link url",
			env: map[string]string{
				"AUTH_PASSWORDLESS_EMAIL_ENABLED": "true",
				"AUTH_MAGIC_LINK_BASE_URL":        "   ",
			},
		},
		{
			name: "passwordless email enabled with non-positive ttl",
			env: map[string]string{
				"AUTH_PASSWORDLESS_EMAIL_ENABLED": "true",
				"AUTH_MAGIC_LINK_TTL_SECONDS":     "0",
			},
		},
		{
			name: "passwordless email enabled without smtp host",
			env: map[string]string{
				"AUTH_PASSWORDLESS_EMAIL_ENABLED": "true",
				"SMTP_HOST":                       "",
			},
		},
		{
			name: "smtp port is not a number",
			env: map[string]string{
				"SMTP_PORT": "mail",
			},
		},
		{
			name: "smtp port is too high",
			env: map[string]string{
				"SMTP_PORT": "70000",
			},
		},
		{
			name: "invalid smtp tls mode",
			env: map[string]string{
				"SMTP_TLS_MODE": "opportunistic",
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

func TestLoadRejectsProductionMagicLinkHTTPBaseURL(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://llmgw:llmgw@example.com:5432/llmgw?sslmode=require")
	t.Setenv("PLATFORM_BOOTSTRAP_TOKEN", "prod-bootstrap-token-012345678901")
	t.Setenv("TOKEN_HASH_SECRET", "prod-token-hash-secret-0123456789")
	t.Setenv("SECRET_ENCRYPTION_KEY", "fedcba98765432100123456789abcdef")
	t.Setenv("AUTH_PASSWORDLESS_EMAIL_ENABLED", "true")
	t.Setenv("AUTH_MAGIC_LINK_BASE_URL", "http://console.example.test/login")
	t.Setenv("SMTP_HOST", "smtp.example.test")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "smtp-user")
	t.Setenv("SMTP_PASSWORD", "smtp-password")
	t.Setenv("SMTP_FROM", "login@example.test")

	if _, err := Load(""); err == nil {
		t.Fatal("Load returned nil error, want production magic link HTTPS validation error")
	}
}

func TestLoadRejectsKafkaEnabledWithoutTopic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
platform_bootstrap_token: bootstrap-token
token_hash_secret: token-hash-secret
secret_encryption_key: 0123456789abcdef0123456789abcdef
kafka_enabled: true
kafka_brokers:
  - kafka:9092
kafka_usage_topic: ""
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	if _, err := Load(path); err == nil {
		t.Fatal("Load returned nil error, want missing Kafka topic error")
	}
}

func TestLoadRejectsClickHouseEnabledWithMissingDatabaseOrUsername(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{
			name: "missing database",
			content: `
platform_bootstrap_token: bootstrap-token
token_hash_secret: token-hash-secret
secret_encryption_key: 0123456789abcdef0123456789abcdef
clickhouse_enabled: true
clickhouse_database: ""
clickhouse_username: default
`,
		},
		{
			name: "missing username",
			content: `
platform_bootstrap_token: bootstrap-token
token_hash_secret: token-hash-secret
secret_encryption_key: 0123456789abcdef0123456789abcdef
clickhouse_enabled: true
clickhouse_database: llmgw
clickhouse_username: ""
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatalf("write config file: %v", err)
			}

			if _, err := Load(path); err == nil {
				t.Fatal("Load returned nil error, want invalid ClickHouse config")
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
