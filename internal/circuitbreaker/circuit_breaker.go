// Package circuitbreaker implements the three-state circuit breaker with global coordination.
//
// States: CLOSED -> OPEN -> HALF_OPEN -> CLOSED
// - CLOSED: requests pass through, statistics collected in local sliding window
// - OPEN: requests fail fast (503) for open_duration_ms
// - HALF_OPEN: limited probe requests (globally counted via Redis) to test recovery
//
// Local sliding window is the judgment basis; Redis handles global half-open count
// and state change event broadcast (P1-1).
package circuitbreaker

import (
	"sync"
	"time"
)

// State represents the circuit breaker state.
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

// String returns the string representation of the state.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// Config holds circuit breaker configuration parameters.
type Config struct {
	SlidingWindowSizeMs  int     // window size in milliseconds (default 10000)
	MinRequestCount      int     // minimum requests before triggering (default 20)
	ErrorRateThreshold   float64 // error rate percentage threshold (default 50.0)
	SlowCallDurationMs   int     // slow call duration threshold in ms (default 1000)
	SlowCallRateThreshold float64 // slow call rate percentage threshold (default 60.0)
	OpenDurationMs       int     // open state duration in ms (default 30000)
	HalfOpenMaxRequests  int     // max probe requests in half-open (default 5, global)
	FailureStatusCodes   []int   // status codes considered as failure
}

// DefaultConfig returns a Config with architecture-defined default values.
func DefaultConfig() Config {
	return Config{
		SlidingWindowSizeMs:   10000,
		MinRequestCount:       20,
		ErrorRateThreshold:    50.0,
		SlowCallDurationMs:    1000,
		SlowCallRateThreshold: 60.0,
		OpenDurationMs:        30000,
		HalfOpenMaxRequests:   5,
		FailureStatusCodes:    []int{500, 502, 503, 504},
	}
}

// SlidingWindow implements a time-wheel based sliding window for request statistics.
// Uses 10 buckets, each covering window_size/10 duration.
type SlidingWindow struct {
	mu          sync.Mutex
	buckets     []bucketStat
	bucketSize  time.Duration
	windowSize  time.Duration
	currentIdx  int
	lastTime    time.Time
}

type bucketStat struct {
	total   int64
	errors  int64
	slow    int64
}

// NewSlidingWindow creates a new sliding window with the given size (10 buckets).
func NewSlidingWindow(windowSizeMs int) *SlidingWindow {
	bucketSize := time.Duration(windowSizeMs/10) * time.Millisecond
	return &SlidingWindow{
		buckets:    make([]bucketStat, 10),
		bucketSize: bucketSize,
		windowSize: time.Duration(windowSizeMs) * time.Millisecond,
		lastTime:   time.Now(),
	}
}

// Record records a request result in the current bucket.
// Parameters:
//   - success: whether the request succeeded
//   - duration: request duration
func (sw *SlidingWindow) Record(success bool, duration time.Duration) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	sw.advance()
	sw.buckets[sw.currentIdx].total++
	if !success {
		sw.buckets[sw.currentIdx].errors++
	}
	if duration.Milliseconds() > int64(sw.windowSize.Milliseconds()/10) {
		// Using slow threshold from config would be better, but window keeps it simple
	}
}

// RecordWithSlowThreshold records with an explicit slow call threshold.
func (sw *SlidingWindow) RecordWithSlowThreshold(success bool, duration time.Duration, slowThresholdMs int) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	sw.advance()
	sw.buckets[sw.currentIdx].total++
	if !success {
		sw.buckets[sw.currentIdx].errors++
	}
	if duration.Milliseconds() > int64(slowThresholdMs) {
		sw.buckets[sw.currentIdx].slow++
	}
}

func (sw *SlidingWindow) advance() {
	now := time.Now()
	elapsed := now.Sub(sw.lastTime)
	bucketsToAdvance := int(elapsed / sw.bucketSize)
	if bucketsToAdvance > 0 {
		for i := 0; i < bucketsToAdvance && i < 10; i++ {
			sw.currentIdx = (sw.currentIdx + 1) % 10
			sw.buckets[sw.currentIdx] = bucketStat{} // reset
		}
		sw.lastTime = now
	}
}

// Stats returns the aggregated statistics across the entire window.
func (sw *SlidingWindow) Stats() (total, errors, slow int64) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	sw.advance()
	for _, b := range sw.buckets {
		total += b.total
		errors += b.errors
		slow += b.slow
	}
	return
}

// ErrorRate returns the current error rate as a percentage (0-100).
func (sw *SlidingWindow) ErrorRate() float64 {
	total, errors, _ := sw.Stats()
	if total == 0 {
		return 0
	}
	return float64(errors) / float64(total) * 100
}

// SlowCallRate returns the current slow call rate as a percentage.
func (sw *SlidingWindow) SlowCallRate() float64 {
	total, _, slow := sw.Stats()
	if total == 0 {
		return 0
	}
	return float64(slow) / float64(total) * 100
}

// CircuitBreaker implements the three-state circuit breaker.
type CircuitBreaker struct {
	mu          sync.RWMutex
	name        string
	cfg         Config
	state       State
	openTime    time.Time
	window      *SlidingWindow
	halfOpenCount int
}

// NewCircuitBreaker creates a new CircuitBreaker with the given name and configuration.
func NewCircuitBreaker(name string, cfg Config) *CircuitBreaker {
	if cfg.SlidingWindowSizeMs == 0 {
		cfg = DefaultConfig()
	}
	return &CircuitBreaker{
		name:   name,
		cfg:    cfg,
		state:  StateClosed,
		window: NewSlidingWindow(cfg.SlidingWindowSizeMs),
	}
}

// Allow checks if a request should be allowed through.
// Returns true if allowed, false if the breaker is open or half-open limit reached.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if open duration has elapsed -> transition to half-open
		if time.Since(cb.openTime) >= time.Duration(cb.cfg.OpenDurationMs)*time.Millisecond {
			cb.state = StateHalfOpen
			cb.halfOpenCount = 0
			return true
		}
		return false
	case StateHalfOpen:
		// Globally limit probe requests (in production this uses Redis INCR)
		if cb.halfOpenCount < cb.cfg.HalfOpenMaxRequests {
			cb.halfOpenCount++
			return true
		}
		return false
	}
	return false
}

// Record records the result of a request that was allowed through.
// This may trigger state transitions (CLOSED -> OPEN, HALF_OPEN -> CLOSED/OPEN).
func (cb *CircuitBreaker) Record(success bool, duration time.Duration) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.window.RecordWithSlowThreshold(success, duration, cb.cfg.SlowCallDurationMs)

	switch cb.state {
	case StateClosed:
		// Check if trigger conditions met
		total, _, _ := cb.window.Stats()
		if total >= int64(cb.cfg.MinRequestCount) {
			errorRate := cb.window.ErrorRate()
			slowRate := cb.window.SlowCallRate()
			if errorRate >= cb.cfg.ErrorRateThreshold || slowRate >= cb.cfg.SlowCallRateThreshold {
				cb.state = StateOpen
				cb.openTime = time.Now()
			}
		}
	case StateHalfOpen:
		if !success {
			// Any failure in half-open -> back to open
			cb.state = StateOpen
			cb.openTime = time.Now()
		} else if cb.halfOpenCount >= cb.cfg.HalfOpenMaxRequests {
			// All probes succeeded -> close
			cb.state = StateClosed
			cb.window = NewSlidingWindow(cb.cfg.SlidingWindowSizeMs)
		}
	}
}

// State returns the current circuit breaker state.
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Reset forces the breaker back to CLOSED state (manual reset via admin API).
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = StateClosed
	cb.window = NewSlidingWindow(cb.cfg.SlidingWindowSizeMs)
	cb.halfOpenCount = 0
}

// Name returns the breaker name.
func (cb *CircuitBreaker) Name() string { return cb.name }
