package observability

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry *prometheus.Registry

	requests           *prometheus.CounterVec
	requestDuration    *prometheus.HistogramVec
	providerRequests   *prometheus.CounterVec
	providerErrors     *prometheus.CounterVec
	tokens             *prometheus.CounterVec
	costMicroUSD       *prometheus.CounterVec
	rateLimited        prometheus.Counter
	budgetBlocked      prometheus.Counter
	eventPublishFail   prometheus.Counter
	analyticsWriteFail prometheus.Counter
}

func NewMetrics() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "llmgw_requests_total",
			Help: "Total Gateway requests.",
		}, []string{"status", "provider", "model"}),
		requestDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "llmgw_request_duration_seconds",
			Help:    "Gateway request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		}, []string{"status", "provider", "model"}),
		providerRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "llmgw_provider_requests_total",
			Help: "Total provider requests.",
		}, []string{"provider", "model", "status"}),
		providerErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "llmgw_provider_errors_total",
			Help: "Total provider errors.",
		}, []string{"provider", "model", "code"}),
		tokens: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "llmgw_tokens_total",
			Help: "Total tokens processed by Gateway.",
		}, []string{"model", "type"}),
		costMicroUSD: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "llmgw_cost_micro_usd_total",
			Help: "Total cost in micro USD.",
		}, []string{"model"}),
		rateLimited: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "llmgw_rate_limited_total",
			Help: "Total rate-limited Gateway requests.",
		}),
		budgetBlocked: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "llmgw_budget_blocked_total",
			Help: "Total budget-blocked Gateway requests.",
		}),
		eventPublishFail: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "llmgw_usage_event_publish_failed_total",
			Help: "Total failed usage event publish attempts.",
		}),
		analyticsWriteFail: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "llmgw_analytics_write_failed_total",
			Help: "Total failed analytics write attempts.",
		}),
	}
	m.registry.MustRegister(
		m.requests,
		m.requestDuration,
		m.providerRequests,
		m.providerErrors,
		m.tokens,
		m.costMicroUSD,
		m.rateLimited,
		m.budgetBlocked,
		m.eventPublishFail,
		m.analyticsWriteFail,
	)
	m.requests.WithLabelValues("", "", "").Add(0)
	m.requestDuration.WithLabelValues("", "", "").Observe(0)
	m.providerRequests.WithLabelValues("", "", "").Add(0)
	m.providerErrors.WithLabelValues("", "", "").Add(0)
	m.tokens.WithLabelValues("", "total").Add(0)
	m.costMicroUSD.WithLabelValues("").Add(0)
	return m
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) ObserveGatewayRequest(status string, provider string, model string, duration time.Duration) {
	if m == nil {
		return
	}
	m.requests.WithLabelValues(status, provider, model).Inc()
	m.requestDuration.WithLabelValues(status, provider, model).Observe(duration.Seconds())
}

func (m *Metrics) ObserveProviderRequest(provider string, model string, status string, code string) {
	if m == nil {
		return
	}
	m.providerRequests.WithLabelValues(provider, model, status).Inc()
	if status != "success" {
		m.providerErrors.WithLabelValues(provider, model, code).Inc()
	}
}

func (m *Metrics) AddTokens(model string, prompt int64, completion int64) {
	if m == nil {
		return
	}
	m.tokens.WithLabelValues(model, "prompt").Add(float64(prompt))
	m.tokens.WithLabelValues(model, "completion").Add(float64(completion))
	m.tokens.WithLabelValues(model, "total").Add(float64(prompt + completion))
}

func (m *Metrics) AddCostMicroUSD(model string, amount int64) {
	if m == nil {
		return
	}
	m.costMicroUSD.WithLabelValues(model).Add(float64(amount))
}

func (m *Metrics) IncRateLimited() {
	if m == nil {
		return
	}
	m.rateLimited.Inc()
}

func (m *Metrics) IncBudgetBlocked() {
	if m == nil {
		return
	}
	m.budgetBlocked.Inc()
}

func (m *Metrics) IncUsageEventPublishFailed() {
	if m == nil {
		return
	}
	m.eventPublishFail.Inc()
}

func (m *Metrics) IncAnalyticsWriteFailed() {
	if m == nil {
		return
	}
	m.analyticsWriteFail.Inc()
}
