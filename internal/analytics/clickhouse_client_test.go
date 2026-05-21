package analytics

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestClickHouseClientExecSendsSQL(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Query().Get("database") != "llmgw" {
			t.Fatalf("database query = %q, want llmgw", r.URL.Query().Get("database"))
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "default" || password != "secret" {
			t.Fatalf("basic auth = %q/%q/%v, want default/secret/true", username, password, ok)
		}
		gotBody = readRequestBody(t, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClickHouseClient(t, server.URL)
	if err := client.Exec(context.Background(), "CREATE TABLE usage_events"); err != nil {
		t.Fatalf("Exec returned error: %v", err)
	}
	if gotBody != "CREATE TABLE usage_events" {
		t.Fatalf("body = %q, want SQL", gotBody)
	}
}

func TestClickHouseClientInsertUsageRecordJSONEachRow(t *testing.T) {
	apiKeyID := uuid.New()
	modelID := uuid.New()
	requestID := uuid.New()
	orgID := uuid.New()
	var payload string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload = readRequestBody(t, r)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClickHouseClient(t, server.URL)
	err := client.InsertUsageRecord(context.Background(), UsageRecord{
		RequestID:         requestID,
		OrgID:             orgID,
		APIKeyID:          &apiKeyID,
		ProviderID:        nil,
		ModelID:           &modelID,
		Status:            "success",
		PromptTokens:      20,
		CompletionTokens:  30,
		TotalTokens:       50,
		TotalCostMicroUSD: 8,
		LatencyMS:         123,
		CreatedAt:         time.Date(2026, 5, 21, 10, 11, 12, 345000000, time.UTC),
	})
	if err != nil {
		t.Fatalf("InsertUsageRecord returned error: %v", err)
	}
	if !strings.HasPrefix(payload, "INSERT INTO usage_events FORMAT JSONEachRow\n") {
		t.Fatalf("payload = %q, want JSONEachRow insert", payload)
	}
	var row map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(payload, "INSERT INTO usage_events FORMAT JSONEachRow\n")), &row); err != nil {
		t.Fatalf("decode JSON row: %v", err)
	}
	if row["request_id"] != requestID.String() || row["org_id"] != orgID.String() {
		t.Fatalf("row ids = %+v, want request/org ids", row)
	}
	if row["api_key_id"] != apiKeyID.String() || row["model_id"] != modelID.String() {
		t.Fatalf("row nullable ids = %+v, want api key/model ids", row)
	}
	if row["provider_id"] != nil {
		t.Fatalf("provider_id = %v, want nil", row["provider_id"])
	}
	if row["created_at"] != "2026-05-21 10:11:12.345" {
		t.Fatalf("created_at = %q, want ClickHouse time", row["created_at"])
	}
}

func TestClickHouseClientQueryJSONEachRow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := readRequestBody(t, r)
		if !strings.Contains(query, "FORMAT JSONEachRow") {
			t.Fatalf("query = %q, want FORMAT JSONEachRow", query)
		}
		_, _ = w.Write([]byte(`{"day":"2026-05-20","total_cost_micro_usd":8}` + "\n" + `{"day":"2026-05-21","total_cost_micro_usd":13}` + "\n"))
	}))
	defer server.Close()

	client := newTestClickHouseClient(t, server.URL)
	var rows []map[string]any
	err := client.QueryJSONEachRow(context.Background(), "SELECT day, total_cost_micro_usd FROM usage_events", func(raw json.RawMessage) error {
		var row map[string]any
		if err := json.Unmarshal(raw, &row); err != nil {
			return err
		}
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		t.Fatalf("QueryJSONEachRow returned error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
}

func TestClickHouseClientReturnsErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "syntax error", http.StatusBadRequest)
	}))
	defer server.Close()

	client := newTestClickHouseClient(t, server.URL)
	err := client.Exec(context.Background(), "bad sql")
	if err == nil || !strings.Contains(err.Error(), "clickhouse status 400") {
		t.Fatalf("Exec error = %v, want status error", err)
	}
}

func newTestClickHouseClient(t *testing.T, serverURL string) *ClickHouseClient {
	t.Helper()

	client, err := NewClickHouseClient(ClickHouseConfig{
		URL:      serverURL,
		Database: "llmgw",
		Username: "default",
		Password: "secret",
	})
	if err != nil {
		t.Fatalf("NewClickHouseClient returned error: %v", err)
	}
	return client
}

func readRequestBody(t *testing.T, r *http.Request) string {
	t.Helper()

	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	return string(body)
}
