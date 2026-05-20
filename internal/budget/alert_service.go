package budget

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
)

const (
	AlertDeliverySuccess = "success"
	AlertDeliveryFailed  = "failed"
)

var alertThresholds = []int32{80, 90, 100}

type AlertService struct {
	store  *store.Store
	budget *Service
	client *http.Client
	clock  func() time.Time
}

type AlertOption func(*AlertService)

func WithAlertHTTPClient(client *http.Client) AlertOption {
	return func(s *AlertService) {
		if client != nil {
			s.client = client
		}
	}
}

func WithAlertClock(clock func() time.Time) AlertOption {
	return func(s *AlertService) {
		if clock != nil {
			s.clock = clock
			s.budget.clock = clock
		}
	}
}

type CreateAlertParams struct {
	OrgID         uuid.UUID
	BudgetID      uuid.UUID
	WebhookURL    string
	WebhookSecret string
	Status        string
}

type CheckAndDeliverResult struct {
	Deliveries []db.BudgetAlertDelivery
}

type webhookPayload struct {
	OrgID         uuid.UUID `json:"org_id"`
	BudgetID      uuid.UUID `json:"budget_id"`
	Threshold     int32     `json:"threshold"`
	UsedMicroUSD  int64     `json:"used_micro_usd"`
	LimitMicroUSD int64     `json:"limit_micro_usd"`
	Period        string    `json:"period"`
	Timestamp     time.Time `json:"timestamp"`
}

func NewAlertService(st *store.Store, opts ...AlertOption) *AlertService {
	service := &AlertService{
		store:  st,
		budget: NewService(st.Queries),
		client: http.DefaultClient,
		clock:  func() time.Time { return time.Now().UTC() },
	}
	service.budget.clock = service.clock
	for _, opt := range opts {
		opt(service)
	}
	return service
}

func (s *AlertService) CreateAlert(ctx context.Context, params CreateAlertParams) (db.BudgetAlert, error) {
	if params.OrgID == uuid.Nil {
		return db.BudgetAlert{}, fmt.Errorf("org_id is required")
	}
	if params.BudgetID == uuid.Nil {
		return db.BudgetAlert{}, fmt.Errorf("budget_id is required")
	}
	webhookURL := strings.TrimSpace(params.WebhookURL)
	if err := validateWebhookURL(webhookURL); err != nil {
		return db.BudgetAlert{}, err
	}
	status := params.Status
	if status == "" {
		status = StatusActive
	}
	if status != StatusActive && status != StatusDisabled {
		return db.BudgetAlert{}, fmt.Errorf("status must be active or disabled")
	}
	now := s.clock()
	return s.store.Queries.CreateBudgetAlert(ctx, db.CreateBudgetAlertParams{
		ID:            uuid.New(),
		OrgID:         params.OrgID,
		BudgetID:      params.BudgetID,
		WebhookUrl:    webhookURL,
		WebhookSecret: nullableText(params.WebhookSecret),
		Status:        status,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
}

func (s *AlertService) ListAlerts(ctx context.Context, orgID uuid.UUID) ([]db.BudgetAlert, error) {
	return s.store.Queries.ListBudgetAlerts(ctx, orgID)
}

func (s *AlertService) ListDeliveries(ctx context.Context, orgID uuid.UUID, limit int32) ([]db.BudgetAlertDelivery, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	return s.store.Queries.ListBudgetAlertDeliveries(ctx, db.ListBudgetAlertDeliveriesParams{
		OrgID: orgID,
		Limit: limit,
	})
}

func (s *AlertService) CheckAndDeliver(ctx context.Context, orgID uuid.UUID) (CheckAndDeliverResult, error) {
	alerts, err := s.store.Queries.ListActiveBudgetAlerts(ctx, orgID)
	if err != nil {
		return CheckAndDeliverResult{}, err
	}
	result := CheckAndDeliverResult{Deliveries: []db.BudgetAlertDelivery{}}
	for _, alert := range alerts {
		budgetItem, err := s.store.Queries.GetBudget(ctx, db.GetBudgetParams{
			OrgID: alert.OrgID,
			ID:    alert.BudgetID,
		})
		if err != nil {
			return CheckAndDeliverResult{}, err
		}
		if budgetItem.Status != StatusActive || budgetItem.LimitMicroUsd < 0 {
			continue
		}
		status, err := s.budget.statusForBudget(ctx, budgetItem)
		if err != nil {
			return CheckAndDeliverResult{}, err
		}
		delivered, err := s.store.Queries.ListBudgetAlertDeliveriesByBudgetWindow(ctx, db.ListBudgetAlertDeliveriesByBudgetWindowParams{
			OrgID:             alert.OrgID,
			BudgetID:          alert.BudgetID,
			PeriodWindowStart: status.WindowStart,
		})
		if err != nil {
			return CheckAndDeliverResult{}, err
		}
		deliveredThresholds := make(map[int32]struct{}, len(delivered))
		for _, item := range delivered {
			deliveredThresholds[item.Threshold] = struct{}{}
		}
		for _, threshold := range alertThresholds {
			if _, ok := deliveredThresholds[threshold]; ok {
				continue
			}
			if !thresholdReached(status.UsedMicroUSD, status.Budget.LimitMicroUsd, threshold) {
				continue
			}
			delivery, err := s.deliver(ctx, alert, status, threshold)
			if err != nil {
				return CheckAndDeliverResult{}, err
			}
			result.Deliveries = append(result.Deliveries, delivery)
		}
	}
	return result, nil
}

func (s *AlertService) deliver(ctx context.Context, alert db.BudgetAlert, status Status, threshold int32) (db.BudgetAlertDelivery, error) {
	payload := webhookPayload{
		OrgID:         alert.OrgID,
		BudgetID:      alert.BudgetID,
		Threshold:     threshold,
		UsedMicroUSD:  status.UsedMicroUSD,
		LimitMicroUSD: status.Budget.LimitMicroUsd,
		Period:        status.Budget.Period,
		Timestamp:     s.clock(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return db.BudgetAlertDelivery{}, err
	}

	deliveryStatus := AlertDeliverySuccess
	var httpStatus *int32
	var errorMessage string
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, alert.WebhookUrl, bytes.NewReader(body))
	if err != nil {
		deliveryStatus = AlertDeliveryFailed
		errorMessage = "build webhook request failed"
	} else {
		req.Header.Set("Content-Type", "application/json")
		if alert.WebhookSecret.Valid {
			req.Header.Set("X-LLMGW-Signature", signWebhook(alert.WebhookSecret.String, body))
		}
		resp, err := s.client.Do(req)
		if err != nil {
			deliveryStatus = AlertDeliveryFailed
			errorMessage = "webhook request failed"
		} else {
			defer resp.Body.Close()
			statusCode := int32(resp.StatusCode)
			httpStatus = &statusCode
			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				deliveryStatus = AlertDeliveryFailed
				errorMessage = fmt.Sprintf("webhook returned status %d", resp.StatusCode)
			}
		}
	}

	now := s.clock()
	return s.store.Queries.InsertBudgetAlertDelivery(ctx, db.InsertBudgetAlertDeliveryParams{
		ID:                uuid.New(),
		OrgID:             alert.OrgID,
		BudgetAlertID:     alert.ID,
		BudgetID:          alert.BudgetID,
		Threshold:         threshold,
		Period:            status.Budget.Period,
		PeriodWindowStart: status.WindowStart,
		PeriodWindowEnd:   status.WindowEnd,
		UsedMicroUsd:      status.UsedMicroUSD,
		LimitMicroUsd:     status.Budget.LimitMicroUsd,
		WebhookUrl:        alert.WebhookUrl,
		Status:            deliveryStatus,
		HttpStatus:        int4Value(httpStatus),
		ErrorMessage:      nullableText(errorMessage),
		CreatedAt:         now,
	})
}

func thresholdReached(used int64, limit int64, threshold int32) bool {
	return used*100 >= limit*int64(threshold)
}

func validateWebhookURL(value string) error {
	if value == "" {
		return fmt.Errorf("webhook_url is required")
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return fmt.Errorf("webhook_url must be a valid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("webhook_url must use http or https")
	}
	if parsed.Host == "" {
		return fmt.Errorf("webhook_url must include host")
	}
	return nil
}

func signWebhook(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func nullableText(value string) pgtype.Text {
	if strings.TrimSpace(value) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(value), Valid: true}
}

func int4Value(value *int32) pgtype.Int4 {
	if value == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: *value, Valid: true}
}
