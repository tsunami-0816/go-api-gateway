package circuitbreaker

import (
	"sync"

	"github.com/go-api-gateway/gateway/internal/model"
)

// CircuitBreakerManager manages multiple circuit breaker instances keyed by policy ID.
// It is synchronized with the config snapshot via Sync() on hot reload.
type CircuitBreakerManager struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker // key: policy ID
}

// NewCircuitBreakerManager creates a new empty manager.
func NewCircuitBreakerManager() *CircuitBreakerManager {
	return &CircuitBreakerManager{
		breakers: make(map[string]*CircuitBreaker),
	}
}

// Sync reconciles internal breakers with the latest snapshot policies.
// - New policies: create breaker instances
// - Changed policies: replace with new config (resets state)
// - Removed policies: delete instances
func (m *CircuitBreakerManager) Sync(policies []*model.CircuitBreaker) {
	m.mu.Lock()
	defer m.mu.Unlock()

	active := make(map[string]bool, len(policies))
	for _, policy := range policies {
		active[policy.ID] = true
		existing, ok := m.breakers[policy.ID]
		if !ok || !policyEqual(existing, policy) {
			m.breakers[policy.ID] = NewCircuitBreaker(policy.PolicyName, modelToConfig(policy))
		}
	}
	for id := range m.breakers {
		if !active[id] {
			delete(m.breakers, id)
		}
	}
}

// Get returns the breaker for the given policy ID, or nil if not found.
func (m *CircuitBreakerManager) Get(policyID string) *CircuitBreaker {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.breakers[policyID]
}

// GetByRouteID returns the first breaker whose policy matches the route ID.
func (m *CircuitBreakerManager) GetByRouteID(routeID string, policies []*model.CircuitBreaker) *CircuitBreaker {
	for _, p := range policies {
		if p.RouteID == routeID {
			return m.Get(p.ID)
		}
	}
	return nil
}

func modelToConfig(p *model.CircuitBreaker) Config {
	cfg := DefaultConfig()
	if p.SlidingWindowSizeMs > 0 {
		cfg.SlidingWindowSizeMs = p.SlidingWindowSizeMs
	}
	if p.MinRequestCount > 0 {
		cfg.MinRequestCount = p.MinRequestCount
	}
	if p.ErrorRateThreshold > 0 {
		cfg.ErrorRateThreshold = p.ErrorRateThreshold
	}
	if p.SlowCallDurationMs > 0 {
		cfg.SlowCallDurationMs = p.SlowCallDurationMs
	}
	if p.SlowCallRateThreshold > 0 {
		cfg.SlowCallRateThreshold = p.SlowCallRateThreshold
	}
	if p.OpenDurationMs > 0 {
		cfg.OpenDurationMs = p.OpenDurationMs
	}
	if p.HalfOpenMaxRequests > 0 {
		cfg.HalfOpenMaxRequests = p.HalfOpenMaxRequests
	}
	return cfg
}

func policyEqual(cb *CircuitBreaker, p *model.CircuitBreaker) bool {
	return cb.cfg.SlidingWindowSizeMs == p.SlidingWindowSizeMs &&
		cb.cfg.MinRequestCount == p.MinRequestCount &&
		cb.cfg.ErrorRateThreshold == p.ErrorRateThreshold &&
		cb.cfg.SlowCallDurationMs == p.SlowCallDurationMs &&
		cb.cfg.SlowCallRateThreshold == p.SlowCallRateThreshold &&
		cb.cfg.OpenDurationMs == p.OpenDurationMs &&
		cb.cfg.HalfOpenMaxRequests == p.HalfOpenMaxRequests
}
