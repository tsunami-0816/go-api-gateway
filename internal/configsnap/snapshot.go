// Package configsnap provides immutable configuration snapshots for the data plane.
//
// The snapshot holds all enabled routes, rate limits, circuit breakers, gray rules,
// and services. It is built from the database and replaced atomically via atomic.Value
// on config hot reload, enabling lock-free reads on the request path.
package configsnap

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/go-api-gateway/gateway/internal/model"
	"github.com/go-api-gateway/gateway/internal/repository"
)

// Snapshot is an immutable configuration snapshot used by the data plane.
// All fields are read-only; modifications require building a new snapshot.
type Snapshot struct {
	Version         int64
	Routes          []*model.Route
	RateLimits      []*model.RateLimit
	CircuitBreakers []*model.CircuitBreaker
	GrayReleases    []*model.GrayRelease
	Services        []*model.Service
}

// ReloadCallback is invoked after a new snapshot is atomically stored.
// Implementations must be non-blocking; heavy work should be dispatched
// to a goroutine or use the snapshot reference directly.
type ReloadCallback func(snap *Snapshot)

// Manager manages atomic snapshot replacement.
type Manager struct {
	snapshot  atomic.Value // stores *Snapshot
	mu        sync.Mutex
	version   int64
	callbacks []ReloadCallback

	routeRepo *repository.RouteRepository
	rlRepo    *repository.RateLimitRepository
	cbRepo    *repository.CircuitBreakerRepository
	grayRepo  *repository.GrayReleaseRepository
	svcRepo   *repository.ServiceRepository
}

// NewManager creates a new snapshot manager with the given repositories.
func NewManager(
	routeRepo *repository.RouteRepository,
	rlRepo *repository.RateLimitRepository,
	cbRepo *repository.CircuitBreakerRepository,
	grayRepo *repository.GrayReleaseRepository,
	svcRepo *repository.ServiceRepository,
) *Manager {
	m := &Manager{
		routeRepo: routeRepo,
		rlRepo:    rlRepo,
		cbRepo:    cbRepo,
		grayRepo:  grayRepo,
		svcRepo:   svcRepo,
	}
	m.snapshot.Store(&Snapshot{})
	return m
}

// Reload builds a new snapshot from the database and atomically replaces it.
// Returns the new snapshot version number.
func (m *Manager) Reload(ctx context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	routes, err := m.routeRepo.ListEnabled(ctx)
	if err != nil {
		return 0, err
	}
	rateLimits, err := m.rlRepo.ListEnabled(ctx)
	if err != nil {
		return 0, err
	}
	circuitBreakers, err := m.cbRepo.ListEnabled(ctx)
	if err != nil {
		return 0, err
	}
	grayReleases, err := m.grayRepo.ListEnabled(ctx)
	if err != nil {
		return 0, err
	}
	services, _, err := m.svcRepo.List(ctx, 1, 10000)
	if err != nil {
		return 0, err
	}

	m.version++
	snap := &Snapshot{
		Version:         m.version,
		Routes:          routes,
		RateLimits:      rateLimits,
		CircuitBreakers: circuitBreakers,
		GrayReleases:    grayReleases,
		Services:        services,
	}
	m.snapshot.Store(snap)
	// Notify all registered callbacks after atomic store.
	// Callbacks run synchronously under the lock; keep them lightweight.
	for _, cb := range m.callbacks {
		cb(snap)
	}
	return m.version, nil
}

// OnReload registers a callback invoked after every successful reload.
// Must be called before the first Reload (typically during app startup).
func (m *Manager) OnReload(cb ReloadCallback) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks = append(m.callbacks, cb)
}

// Get returns the current configuration snapshot (lock-free read).
func (m *Manager) Get() *Snapshot {
	return m.snapshot.Load().(*Snapshot)
}

// GetRouteByID returns a route from the snapshot by ID.
func (s *Snapshot) GetRouteByID(id string) *model.Route {
	for _, r := range s.Routes {
		if r.ID == id {
			return r
		}
	}
	return nil
}

// GetGrayRuleByRouteID returns the gray release rule for a route.
func (s *Snapshot) GetGrayRuleByRouteID(routeID string) *model.GrayRelease {
	for _, g := range s.GrayReleases {
		if g.RouteID == routeID {
			return g
		}
	}
	return nil
}

// GetRateLimitsByRouteID returns all rate limit rules for a route.
func (s *Snapshot) GetRateLimitsByRouteID(routeID string) []*model.RateLimit {
	var result []*model.RateLimit
	for _, rl := range s.RateLimits {
		if rl.RouteID == routeID {
			result = append(result, rl)
		}
	}
	return result
}

// GetCircuitBreakerByRouteID returns the circuit breaker policy for a route.
func (s *Snapshot) GetCircuitBreakerByRouteID(routeID string) *model.CircuitBreaker {
	for _, cb := range s.CircuitBreakers {
		if cb.RouteID == routeID {
			return cb
		}
	}
	return nil
}
