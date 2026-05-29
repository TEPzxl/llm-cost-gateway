package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const aes256KeyLength = 32

type Config struct {
	AppEnv                     string   `yaml:"app_env"`
	ServerPort                 int      `yaml:"server_port"`
	DatabaseURL                string   `yaml:"database_url"`
	RedisURL                   string   `yaml:"redis_url"`
	PlatformBootstrapToken     string   `yaml:"platform_bootstrap_token"`
	TokenHashSecret            string   `yaml:"token_hash_secret"`
	TokenHashSecretVersion     int      `yaml:"token_hash_secret_version"`
	TokenHashSecretKeyring     string   `yaml:"token_hash_secret_keyring"`
	SecretEncryptionKey        string   `yaml:"secret_encryption_key"`
	SecretEncryptionKeyVersion int      `yaml:"secret_encryption_key_version"`
	SecretEncryptionKeyring    string   `yaml:"secret_encryption_keyring"`
	LogLevel                   string   `yaml:"log_level"`
	MaxRetries                 int      `yaml:"max_retries"`
	RetryBackoffMS             int      `yaml:"retry_backoff_ms"`
	KafkaEnabled               bool     `yaml:"kafka_enabled"`
	KafkaBrokers               []string `yaml:"kafka_brokers"`
	KafkaUsageTopic            string   `yaml:"kafka_usage_topic"`
	ClickHouseEnabled          bool     `yaml:"clickhouse_enabled"`
	ClickHouseURL              string   `yaml:"clickhouse_url"`
	ClickHouseDatabase         string   `yaml:"clickhouse_database"`
	ClickHouseUsername         string   `yaml:"clickhouse_username"`
	ClickHousePassword         string   `yaml:"clickhouse_password"`
	PromptCacheEnabled         bool     `yaml:"prompt_cache_enabled"`
	PromptCacheTTLSeconds      int      `yaml:"prompt_cache_ttl_seconds"`
	SemanticCacheEnabled       bool     `yaml:"semantic_cache_enabled"`
	SemanticCacheThreshold     float64  `yaml:"semantic_cache_threshold"`
	SemanticCacheMaxTemp       float64  `yaml:"semantic_cache_max_temperature"`
	TracingEnabled             bool     `yaml:"tracing_enabled"`
	TracingOTLPEndpoint        string   `yaml:"tracing_otlp_endpoint"`
	TracingInsecure            bool     `yaml:"tracing_insecure"`
	TracingServiceName         string   `yaml:"tracing_service_name"`
	OutboundPublicOnly         bool     `yaml:"outbound_public_only"`
	PasswordlessEmailEnabled   bool     `yaml:"auth_passwordless_email_enabled"`
	MagicLinkBaseURL           string   `yaml:"auth_magic_link_base_url"`
	MagicLinkTTLSeconds        int      `yaml:"auth_magic_link_ttl_seconds"`
	SMTPHost                   string   `yaml:"smtp_host"`
	SMTPPort                   int      `yaml:"smtp_port"`
	SMTPUsername               string   `yaml:"smtp_username"`
	SMTPPassword               string   `yaml:"smtp_password"`
	SMTPFrom                   string   `yaml:"smtp_from"`
	SMTPTLSMode                string   `yaml:"smtp_tls_mode"`
}

func Load(path string) (Config, error) {
	cfg := defaultConfig()

	if path == "" {
		path = os.Getenv("CONFIG_FILE")
	}
	if path != "" {
		if err := loadFile(path, &cfg); err != nil {
			return Config{}, err
		}
	}

	if err := applyEnv(&cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if !validAppEnv(c.AppEnv) {
		return fmt.Errorf("APP_ENV must be one of development, test, production")
	}
	if c.PlatformBootstrapToken == "" {
		return errors.New("PLATFORM_BOOTSTRAP_TOKEN is required")
	}
	if c.TokenHashSecret == "" {
		return errors.New("TOKEN_HASH_SECRET is required")
	}
	if c.TokenHashSecretVersion <= 0 {
		return fmt.Errorf("TOKEN_HASH_SECRET_VERSION must be greater than 0")
	}
	if _, err := c.TokenHashSecrets(); err != nil {
		return err
	}
	if c.SecretEncryptionKey == "" {
		return errors.New("SECRET_ENCRYPTION_KEY is required")
	}
	if len(c.SecretEncryptionKey) != aes256KeyLength {
		return fmt.Errorf("SECRET_ENCRYPTION_KEY must be %d bytes for AES-256", aes256KeyLength)
	}
	if c.SecretEncryptionKeyVersion <= 0 {
		return fmt.Errorf("SECRET_ENCRYPTION_KEY_VERSION must be greater than 0")
	}
	if _, err := c.SecretEncryptionKeys(); err != nil {
		return err
	}
	if c.ServerPort <= 0 || c.ServerPort > 65535 {
		return fmt.Errorf("SERVER_PORT must be between 1 and 65535")
	}
	if err := validatePostgresURL(c.DatabaseURL); err != nil {
		return err
	}
	if err := validateRedisURL(c.RedisURL); err != nil {
		return err
	}
	if !validLogLevel(c.LogLevel) {
		return fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error, dpanic, panic, fatal")
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("MAX_RETRIES must be greater than or equal to 0")
	}
	if c.RetryBackoffMS <= 0 {
		return fmt.Errorf("RETRY_BACKOFF_MS must be greater than 0")
	}
	if c.KafkaEnabled && len(c.KafkaBrokers) == 0 {
		return fmt.Errorf("KAFKA_BROKERS is required when Kafka is enabled")
	}
	if c.KafkaEnabled && c.KafkaUsageTopic == "" {
		return fmt.Errorf("KAFKA_USAGE_TOPIC is required when Kafka is enabled")
	}
	if c.ClickHouseEnabled {
		if err := validateHTTPURL("CLICKHOUSE_URL", c.ClickHouseURL); err != nil {
			return err
		}
		if c.ClickHouseDatabase == "" {
			return fmt.Errorf("CLICKHOUSE_DATABASE is required when ClickHouse is enabled")
		}
		if c.ClickHouseUsername == "" {
			return fmt.Errorf("CLICKHOUSE_USERNAME is required when ClickHouse is enabled")
		}
	}
	if c.PromptCacheTTLSeconds <= 0 {
		return fmt.Errorf("PROMPT_CACHE_TTL_SECONDS must be greater than 0")
	}
	if c.SemanticCacheThreshold <= 0 || c.SemanticCacheThreshold > 1 {
		return fmt.Errorf("SEMANTIC_CACHE_THRESHOLD must be greater than 0 and less than or equal to 1")
	}
	if c.SemanticCacheMaxTemp < 0 {
		return fmt.Errorf("SEMANTIC_CACHE_MAX_TEMPERATURE must be greater than or equal to 0")
	}
	if c.TracingEnabled {
		if strings.TrimSpace(c.TracingOTLPEndpoint) == "" {
			return fmt.Errorf("TRACING_OTLP_ENDPOINT is required when tracing is enabled")
		}
		if strings.TrimSpace(c.TracingServiceName) == "" {
			return fmt.Errorf("TRACING_SERVICE_NAME is required when tracing is enabled")
		}
	}
	if c.MagicLinkTTLSeconds <= 0 {
		return fmt.Errorf("AUTH_MAGIC_LINK_TTL_SECONDS must be greater than 0")
	}
	if c.SMTPPort <= 0 || c.SMTPPort > 65535 {
		return fmt.Errorf("SMTP_PORT must be between 1 and 65535")
	}
	if !validSMTPTLSMode(c.SMTPTLSMode) {
		return fmt.Errorf("SMTP_TLS_MODE must be one of none, starttls, implicit")
	}
	if c.PasswordlessEmailEnabled {
		if err := validateHTTPURL("AUTH_MAGIC_LINK_BASE_URL", c.MagicLinkBaseURL); err != nil {
			return err
		}
		if strings.TrimSpace(c.SMTPHost) == "" {
			return fmt.Errorf("SMTP_HOST is required when passwordless email is enabled")
		}
		if strings.TrimSpace(c.SMTPFrom) == "" {
			return fmt.Errorf("SMTP_FROM is required when passwordless email is enabled")
		}
	}
	if c.AppEnv == "production" {
		if err := validateProductionConfig(c); err != nil {
			return err
		}
	}
	return nil
}

func defaultConfig() Config {
	return Config{
		AppEnv:                     "development",
		ServerPort:                 8080,
		DatabaseURL:                "postgres://llmgw:llmgw@localhost:5432/llmgw?sslmode=disable",
		RedisURL:                   "redis://localhost:6379/0",
		TokenHashSecretVersion:     1,
		SecretEncryptionKeyVersion: 1,
		LogLevel:                   "info",
		MaxRetries:                 1,
		RetryBackoffMS:             100,
		KafkaEnabled:               false,
		KafkaBrokers:               []string{},
		KafkaUsageTopic:            "llm-usage-events",
		ClickHouseEnabled:          false,
		ClickHouseURL:              "http://localhost:8123",
		ClickHouseDatabase:         "llmgw",
		ClickHouseUsername:         "default",
		PromptCacheEnabled:         true,
		PromptCacheTTLSeconds:      300,
		SemanticCacheEnabled:       false,
		SemanticCacheThreshold:     0.92,
		SemanticCacheMaxTemp:       0.3,
		TracingEnabled:             false,
		TracingOTLPEndpoint:        "localhost:4318",
		TracingInsecure:            true,
		TracingServiceName:         "llm-cost-gateway",
		OutboundPublicOnly:         true,
		PasswordlessEmailEnabled:   false,
		MagicLinkBaseURL:           "http://localhost:3000",
		MagicLinkTTLSeconds:        900,
		SMTPPort:                   587,
		SMTPTLSMode:                "starttls",
	}
}

func loadFile(path string, cfg *Config) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}
	if err := yaml.Unmarshal(content, cfg); err != nil {
		return fmt.Errorf("parse config file: %w", err)
	}
	return nil
}

func applyEnv(cfg *Config) error {
	setStringFromEnv("APP_ENV", &cfg.AppEnv)
	setStringFromEnv("DATABASE_URL", &cfg.DatabaseURL)
	setStringFromEnv("REDIS_URL", &cfg.RedisURL)
	setStringFromEnv("PLATFORM_BOOTSTRAP_TOKEN", &cfg.PlatformBootstrapToken)
	setStringFromEnv("TOKEN_HASH_SECRET", &cfg.TokenHashSecret)
	if value := os.Getenv("TOKEN_HASH_SECRET_VERSION"); value != "" {
		version, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse TOKEN_HASH_SECRET_VERSION: %w", err)
		}
		cfg.TokenHashSecretVersion = version
	}
	setStringFromEnv("TOKEN_HASH_SECRET_KEYRING", &cfg.TokenHashSecretKeyring)
	setStringFromEnv("SECRET_ENCRYPTION_KEY", &cfg.SecretEncryptionKey)
	if value := os.Getenv("SECRET_ENCRYPTION_KEY_VERSION"); value != "" {
		version, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse SECRET_ENCRYPTION_KEY_VERSION: %w", err)
		}
		cfg.SecretEncryptionKeyVersion = version
	}
	setStringFromEnv("SECRET_ENCRYPTION_KEYRING", &cfg.SecretEncryptionKeyring)
	setStringFromEnv("LOG_LEVEL", &cfg.LogLevel)

	if value := os.Getenv("SERVER_PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse SERVER_PORT: %w", err)
		}
		cfg.ServerPort = port
	}
	if value := os.Getenv("MAX_RETRIES"); value != "" {
		maxRetries, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse MAX_RETRIES: %w", err)
		}
		cfg.MaxRetries = maxRetries
	}
	if value := os.Getenv("RETRY_BACKOFF_MS"); value != "" {
		retryBackoffMS, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse RETRY_BACKOFF_MS: %w", err)
		}
		cfg.RetryBackoffMS = retryBackoffMS
	}
	if value := os.Getenv("KAFKA_ENABLED"); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse KAFKA_ENABLED: %w", err)
		}
		cfg.KafkaEnabled = enabled
	}
	if value := os.Getenv("KAFKA_BROKERS"); value != "" {
		cfg.KafkaBrokers = splitCSV(value)
	}
	setStringFromEnv("KAFKA_USAGE_TOPIC", &cfg.KafkaUsageTopic)
	if value := os.Getenv("CLICKHOUSE_ENABLED"); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse CLICKHOUSE_ENABLED: %w", err)
		}
		cfg.ClickHouseEnabled = enabled
	}
	setStringFromEnv("CLICKHOUSE_URL", &cfg.ClickHouseURL)
	setStringFromEnv("CLICKHOUSE_DATABASE", &cfg.ClickHouseDatabase)
	setStringFromEnv("CLICKHOUSE_USERNAME", &cfg.ClickHouseUsername)
	setStringFromEnv("CLICKHOUSE_PASSWORD", &cfg.ClickHousePassword)
	if value := os.Getenv("PROMPT_CACHE_ENABLED"); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse PROMPT_CACHE_ENABLED: %w", err)
		}
		cfg.PromptCacheEnabled = enabled
	}
	if value := os.Getenv("PROMPT_CACHE_TTL_SECONDS"); value != "" {
		ttl, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse PROMPT_CACHE_TTL_SECONDS: %w", err)
		}
		cfg.PromptCacheTTLSeconds = ttl
	}
	if value := os.Getenv("SEMANTIC_CACHE_ENABLED"); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse SEMANTIC_CACHE_ENABLED: %w", err)
		}
		cfg.SemanticCacheEnabled = enabled
	}
	if value := os.Getenv("SEMANTIC_CACHE_THRESHOLD"); value != "" {
		threshold, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("parse SEMANTIC_CACHE_THRESHOLD: %w", err)
		}
		cfg.SemanticCacheThreshold = threshold
	}
	if value := os.Getenv("SEMANTIC_CACHE_MAX_TEMPERATURE"); value != "" {
		maxTemp, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("parse SEMANTIC_CACHE_MAX_TEMPERATURE: %w", err)
		}
		cfg.SemanticCacheMaxTemp = maxTemp
	}
	if value := os.Getenv("TRACING_ENABLED"); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse TRACING_ENABLED: %w", err)
		}
		cfg.TracingEnabled = enabled
	}
	setStringFromEnv("TRACING_OTLP_ENDPOINT", &cfg.TracingOTLPEndpoint)
	if value := os.Getenv("TRACING_INSECURE"); value != "" {
		insecure, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse TRACING_INSECURE: %w", err)
		}
		cfg.TracingInsecure = insecure
	}
	setStringFromEnv("TRACING_SERVICE_NAME", &cfg.TracingServiceName)
	if value := os.Getenv("OUTBOUND_PUBLIC_ONLY"); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse OUTBOUND_PUBLIC_ONLY: %w", err)
		}
		cfg.OutboundPublicOnly = enabled
	}
	if value := os.Getenv("AUTH_PASSWORDLESS_EMAIL_ENABLED"); value != "" {
		enabled, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("parse AUTH_PASSWORDLESS_EMAIL_ENABLED: %w", err)
		}
		cfg.PasswordlessEmailEnabled = enabled
	}
	setStringFromEnv("AUTH_MAGIC_LINK_BASE_URL", &cfg.MagicLinkBaseURL)
	if value := os.Getenv("AUTH_MAGIC_LINK_TTL_SECONDS"); value != "" {
		ttl, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse AUTH_MAGIC_LINK_TTL_SECONDS: %w", err)
		}
		cfg.MagicLinkTTLSeconds = ttl
	}
	setStringFromEnv("SMTP_HOST", &cfg.SMTPHost)
	if value := os.Getenv("SMTP_PORT"); value != "" {
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("parse SMTP_PORT: %w", err)
		}
		cfg.SMTPPort = port
	}
	setStringFromEnv("SMTP_USERNAME", &cfg.SMTPUsername)
	setStringFromEnv("SMTP_PASSWORD", &cfg.SMTPPassword)
	setStringFromEnv("SMTP_FROM", &cfg.SMTPFrom)
	setStringFromEnv("SMTP_TLS_MODE", &cfg.SMTPTLSMode)
	return nil
}

func setStringFromEnv(name string, target *string) {
	if value := os.Getenv(name); value != "" {
		*target = value
	}
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func (c Config) SecretEncryptionKeys() (map[int32]string, error) {
	version := c.SecretEncryptionKeyVersion
	if version <= 0 {
		version = 1
	}
	if strings.TrimSpace(c.SecretEncryptionKeyring) == "" {
		return map[int32]string{int32(version): c.SecretEncryptionKey}, nil
	}
	entries := splitCSV(c.SecretEncryptionKeyring)
	keys := make(map[int32]string, len(entries))
	for _, entry := range entries {
		versionText, key, ok := strings.Cut(entry, ":")
		if !ok {
			return nil, fmt.Errorf("SECRET_ENCRYPTION_KEYRING entries must use version:key format")
		}
		version, err := strconv.Atoi(strings.TrimSpace(versionText))
		if err != nil {
			return nil, fmt.Errorf("parse SECRET_ENCRYPTION_KEYRING version: %w", err)
		}
		if version <= 0 {
			return nil, fmt.Errorf("SECRET_ENCRYPTION_KEYRING version must be greater than 0")
		}
		trimmedKey := strings.TrimSpace(key)
		if len(trimmedKey) != aes256KeyLength {
			return nil, fmt.Errorf("SECRET_ENCRYPTION_KEYRING key version %d must be %d bytes for AES-256", version, aes256KeyLength)
		}
		keys[int32(version)] = trimmedKey
	}
	if _, ok := keys[int32(version)]; !ok {
		return nil, fmt.Errorf("SECRET_ENCRYPTION_KEY_VERSION must exist in SECRET_ENCRYPTION_KEYRING")
	}
	return keys, nil
}

func (c Config) TokenHashSecrets() (map[int32]string, error) {
	version := c.TokenHashSecretVersion
	if version <= 0 {
		version = 1
	}
	if strings.TrimSpace(c.TokenHashSecretKeyring) == "" {
		return map[int32]string{int32(version): c.TokenHashSecret}, nil
	}
	entries := splitCSV(c.TokenHashSecretKeyring)
	secrets := make(map[int32]string, len(entries))
	for _, entry := range entries {
		versionText, secret, ok := strings.Cut(entry, ":")
		if !ok {
			return nil, fmt.Errorf("TOKEN_HASH_SECRET_KEYRING entries must use version:secret format")
		}
		parsedVersion, err := strconv.Atoi(strings.TrimSpace(versionText))
		if err != nil {
			return nil, fmt.Errorf("parse TOKEN_HASH_SECRET_KEYRING version: %w", err)
		}
		if parsedVersion <= 0 {
			return nil, fmt.Errorf("TOKEN_HASH_SECRET_KEYRING version must be greater than 0")
		}
		trimmedSecret := strings.TrimSpace(secret)
		if trimmedSecret == "" {
			return nil, fmt.Errorf("TOKEN_HASH_SECRET_KEYRING secret version %d is empty", parsedVersion)
		}
		secrets[int32(parsedVersion)] = trimmedSecret
	}
	if _, ok := secrets[int32(version)]; !ok {
		return nil, fmt.Errorf("TOKEN_HASH_SECRET_VERSION must exist in TOKEN_HASH_SECRET_KEYRING")
	}
	return secrets, nil
}

func validAppEnv(value string) bool {
	switch value {
	case "development", "test", "production":
		return true
	default:
		return false
	}
}

func validLogLevel(value string) bool {
	switch value {
	case "debug", "info", "warn", "error", "dpanic", "panic", "fatal":
		return true
	default:
		return false
	}
}

func validSMTPTLSMode(value string) bool {
	switch value {
	case "none", "starttls", "implicit":
		return true
	default:
		return false
	}
}

func validatePostgresURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("DATABASE_URL must be a valid URL: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return fmt.Errorf("DATABASE_URL must use postgres or postgresql scheme")
	}
	if parsed.Host == "" {
		return fmt.Errorf("DATABASE_URL must include host")
	}
	return nil
}

func validateProductionConfig(c Config) error {
	if weakProductionSecret(c.PlatformBootstrapToken, "change-me-bootstrap-token", "dev-bootstrap-token") {
		return fmt.Errorf("PLATFORM_BOOTSTRAP_TOKEN must be a strong production secret")
	}
	if weakProductionSecret(c.TokenHashSecret, "change-me-token-hash-secret", "dev-token-hash-secret") {
		return fmt.Errorf("TOKEN_HASH_SECRET must be a strong production secret")
	}
	if c.SecretEncryptionKey == "0123456789abcdef0123456789abcdef" {
		return fmt.Errorf("SECRET_ENCRYPTION_KEY must not use the example key in production")
	}
	parsed, err := url.Parse(c.DatabaseURL)
	if err != nil {
		return fmt.Errorf("DATABASE_URL must be a valid URL: %w", err)
	}
	if strings.EqualFold(parsed.Query().Get("sslmode"), "disable") {
		return fmt.Errorf("DATABASE_URL must not use sslmode=disable in production")
	}
	if c.PasswordlessEmailEnabled {
		magicLinkURL, err := url.Parse(c.MagicLinkBaseURL)
		if err != nil {
			return fmt.Errorf("AUTH_MAGIC_LINK_BASE_URL must be a valid URL: %w", err)
		}
		if magicLinkURL.Scheme != "https" {
			return fmt.Errorf("AUTH_MAGIC_LINK_BASE_URL must use https in production")
		}
	}
	return nil
}

func weakProductionSecret(value string, forbidden ...string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < 32 {
		return true
	}
	for _, item := range forbidden {
		if trimmed == item {
			return true
		}
	}
	return false
}

func validateRedisURL(value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("REDIS_URL must be a valid URL: %w", err)
	}
	if parsed.Scheme != "redis" && parsed.Scheme != "rediss" {
		return fmt.Errorf("REDIS_URL must use redis or rediss scheme")
	}
	if parsed.Host == "" {
		return fmt.Errorf("REDIS_URL must include host")
	}
	return nil
}

func validateHTTPURL(name string, value string) error {
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %w", name, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%s must use http or https scheme", name)
	}
	if parsed.Host == "" {
		return fmt.Errorf("%s must include host", name)
	}
	return nil
}
