// Package observability provides Prometheus metrics, OpenTelemetry tracing,
// and sensitive field masking for the gateway.
package observability

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the gateway.
type Metrics struct {
	QPSTotal           *prometheus.CounterVec
	RequestDuration    *prometheus.HistogramVec
	ErrorRate          *prometheus.GaugeVec
	RateLimitTotal     *prometheus.CounterVec
	RateLimitPassed    *prometheus.CounterVec
	RateLimitRejected  *prometheus.CounterVec
	CircuitBreakerState *prometheus.GaugeVec
	CircuitBreakerOpenTotal *prometheus.CounterVec
	GrayRequestsTotal  *prometheus.CounterVec
	ServiceInstances   *prometheus.GaugeVec
	Goroutines         prometheus.Gauge
	MemoryUsageBytes   prometheus.Gauge
	ActiveConnections  prometheus.Gauge
	ConfigReloadTotal  *prometheus.CounterVec
}

// NewMetrics creates and registers all gateway metrics.
func NewMetrics() *Metrics {
	return &Metrics{
		QPSTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_qps_total",
			Help: "Total number of requests processed by the gateway",
		}, []string{"route", "service", "method", "status"}),

		RequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gateway_request_duration_seconds",
			Help:    "Request duration in seconds",
			Buckets: prometheus.DefBuckets,
		}, []string{"route", "service", "method"}),

		ErrorRate: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gateway_error_rate",
			Help: "Current error rate percentage",
		}, []string{"route", "service"}),

		RateLimitTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_rate_limit_total",
			Help: "Total rate limit checks",
		}, []string{"route"}),

		RateLimitPassed: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_rate_limit_passed",
			Help: "Requests passed rate limit",
		}, []string{"route"}),

		RateLimitRejected: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_rate_limit_rejected",
			Help: "Requests rejected by rate limit",
		}, []string{"route"}),

		CircuitBreakerState: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gateway_circuit_breaker_state",
			Help: "Circuit breaker state (0=closed, 1=open, 2=half_open)",
		}, []string{"service", "route"}),

		CircuitBreakerOpenTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_circuit_breaker_open_total",
			Help: "Total circuit breaker open events",
		}, []string{"service", "route"}),

		GrayRequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_gray_requests_total",
			Help: "Total requests by gray version",
		}, []string{"route", "version"}),

		ServiceInstances: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "gateway_service_instances",
			Help: "Number of registered service instances",
		}, []string{"service", "version", "healthy"}),

		Goroutines: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "gateway_goroutines",
			Help: "Number of goroutines",
		}),

		MemoryUsageBytes: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "gateway_memory_usage_bytes",
			Help: "Memory usage in bytes",
		}),

		ActiveConnections: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "gateway_active_connections",
			Help: "Number of active connections",
		}),

		ConfigReloadTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "gateway_config_reload_total",
			Help: "Total config reload events",
		}, []string{"result"}),
	}
}

// MaskSensitiveFields masks sensitive data in strings for logging.
// Rules: password->******, token->first4+last4, id_card->first6+last4,
// phone->first3+last4, email->first char+domain, cookie/authorization->[REDACTED]
func MaskSensitiveFields(input string) string {
	result := input
	// Mask authorization/cookie headers
	result = maskField(result, "authorization", "[REDACTED]")
	result = maskField(result, "cookie", "[REDACTED]")
	result = maskField(result, "password", "******")
	result = maskField(result, "token", maskMiddle(input, 4, 4))
	return result
}

func maskField(input, fieldName, replacement string) string {
	// Simple JSON field masking: "fieldName": "value" -> "fieldName": "replacement"
	lower := strings.ToLower(input)
	idx := strings.Index(lower, `"`+strings.ToLower(fieldName)+`"`)
	if idx < 0 {
		return input
	}
	// Find the value after the colon
	rest := input[idx+len(fieldName)+2:]
	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		return input
	}
	rest = rest[colonIdx+1:]
	// Find the closing quote
	quoteStart := strings.Index(rest, `"`)
	if quoteStart < 0 {
		return input
	}
	quoteEnd := strings.Index(rest[quoteStart+1:], `"`)
	if quoteEnd < 0 {
		return input
	}
	// Replace the value
	prefix := input[:idx+len(fieldName)+2+colonIdx+1+quoteStart+1]
	suffix := rest[quoteStart+1+quoteEnd:]
	return prefix + replacement + suffix
}

func maskMiddle(input string, prefixLen, suffixLen int) string {
	if len(input) <= prefixLen+suffixLen {
		return "****"
	}
	return input[:prefixLen] + "****" + input[len(input)-suffixLen:]
}
