package analytics

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const clickHouseTimeLayout = "2006-01-02 15:04:05.000"

type ClickHouseConfig struct {
	URL      string
	Database string
	Username string
	Password string
}

type ClickHouseClient struct {
	endpoint   *url.URL
	database   string
	username   string
	password   string
	httpClient *http.Client
}

type UsageRecord struct {
	RequestID         uuid.UUID
	OrgID             uuid.UUID
	APIKeyID          *uuid.UUID
	ProviderID        *uuid.UUID
	ModelID           *uuid.UUID
	Status            string
	PromptTokens      int64
	CompletionTokens  int64
	TotalTokens       int64
	TotalCostMicroUSD int64
	LatencyMS         int32
	CreatedAt         time.Time
}

func NewClickHouseClient(cfg ClickHouseConfig) (*ClickHouseClient, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("clickhouse url is required")
	}
	endpoint, err := url.Parse(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse clickhouse url: %w", err)
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, fmt.Errorf("clickhouse url must use http or https scheme")
	}
	if endpoint.Host == "" {
		return nil, fmt.Errorf("clickhouse url must include host")
	}
	if cfg.Database == "" {
		return nil, fmt.Errorf("clickhouse database is required")
	}
	if cfg.Username == "" {
		return nil, fmt.Errorf("clickhouse username is required")
	}
	return &ClickHouseClient{
		endpoint:   endpoint,
		database:   cfg.Database,
		username:   cfg.Username,
		password:   cfg.Password,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}, nil
}

func (c *ClickHouseClient) Ping(ctx context.Context) error {
	_, err := c.execSQL(ctx, "SELECT 1")
	return err
}

func (c *ClickHouseClient) Exec(ctx context.Context, query string) error {
	_, err := c.execSQL(ctx, query)
	return err
}

func (c *ClickHouseClient) InsertUsageRecord(ctx context.Context, record UsageRecord) error {
	row := clickHouseUsageRecord{
		RequestID:         record.RequestID.String(),
		OrgID:             record.OrgID.String(),
		APIKeyID:          nullableUUID(record.APIKeyID),
		ProviderID:        nullableUUID(record.ProviderID),
		ModelID:           nullableUUID(record.ModelID),
		Status:            record.Status,
		PromptTokens:      record.PromptTokens,
		CompletionTokens:  record.CompletionTokens,
		TotalTokens:       record.TotalTokens,
		TotalCostMicroUSD: record.TotalCostMicroUSD,
		LatencyMS:         record.LatencyMS,
		CreatedAt:         record.CreatedAt.UTC().Format(clickHouseTimeLayout),
	}
	body, err := json.Marshal(row)
	if err != nil {
		return fmt.Errorf("marshal clickhouse usage record: %w", err)
	}
	query := "INSERT INTO usage_events FORMAT JSONEachRow\n" + string(body)
	_, err = c.execSQL(ctx, query)
	return err
}

func (c *ClickHouseClient) QueryJSONEachRow(ctx context.Context, query string, scan func(json.RawMessage) error) error {
	if !strings.Contains(strings.ToUpper(query), "FORMAT JSONEACHROW") {
		query += "\nFORMAT JSONEachRow"
	}
	body, err := c.execSQL(ctx, query)
	if err != nil {
		return err
	}
	reader := bufio.NewScanner(bytes.NewReader(body))
	for reader.Scan() {
		line := bytes.TrimSpace(reader.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := scan(append(json.RawMessage(nil), line...)); err != nil {
			return err
		}
	}
	if err := reader.Err(); err != nil {
		return fmt.Errorf("read clickhouse response: %w", err)
	}
	return nil
}

func (c *ClickHouseClient) Close() error {
	return nil
}

func (c *ClickHouseClient) execSQL(ctx context.Context, query string) ([]byte, error) {
	if c == nil {
		return nil, fmt.Errorf("clickhouse client is nil")
	}
	requestURL := *c.endpoint
	values := requestURL.Query()
	values.Set("database", c.database)
	values.Set("date_time_input_format", "best_effort")
	values.Set("output_format_json_quote_64bit_integers", "0")
	requestURL.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), strings.NewReader(query))
	if err != nil {
		return nil, fmt.Errorf("create clickhouse request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain; charset=utf-8")
	req.SetBasicAuth(c.username, c.password)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call clickhouse: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("clickhouse status %d: %s", resp.StatusCode, truncateForError(body))
	}
	if readErr != nil {
		return nil, fmt.Errorf("read clickhouse response: %w", readErr)
	}
	return body, nil
}

type clickHouseUsageRecord struct {
	RequestID         string  `json:"request_id"`
	OrgID             string  `json:"org_id"`
	APIKeyID          *string `json:"api_key_id"`
	ProviderID        *string `json:"provider_id"`
	ModelID           *string `json:"model_id"`
	Status            string  `json:"status"`
	PromptTokens      int64   `json:"prompt_tokens"`
	CompletionTokens  int64   `json:"completion_tokens"`
	TotalTokens       int64   `json:"total_tokens"`
	TotalCostMicroUSD int64   `json:"total_cost_micro_usd"`
	LatencyMS         int32   `json:"latency_ms"`
	CreatedAt         string  `json:"created_at"`
}

func nullableUUID(value *uuid.UUID) *string {
	if value == nil {
		return nil
	}
	text := value.String()
	return &text
}

func truncateForError(body []byte) string {
	const max = 512
	text := strings.TrimSpace(string(body))
	if len(text) <= max {
		return text
	}
	return text[:max]
}
