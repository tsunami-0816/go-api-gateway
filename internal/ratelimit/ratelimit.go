// Package ratelimit implements the two-layer rate limiting engine.
//
// Layer 1: in-memory token bucket (nanosecond-level, no network I/O)
// Layer 2: Redis for multi-instance token batch prefetch (background goroutine)
//
// The request path only operates on local memory; Redis sync runs in background.
package ratelimit

import (
	"sync"
	"time"
)

// TokenBucket implements a thread-safe in-memory token bucket algorithm.
type TokenBucket struct {
	mu             sync.Mutex
	tokens         float64   // current token count
	rate           float64   // refill rate (tokens per second)
	capacity       float64   // maximum tokens
	lastRefillTime time.Time // last refill timestamp
}

// NewTokenBucket creates a new token bucket with the given rate and capacity.
//
// Parameters:
//   - rate: tokens added per second
//   - capacity: maximum tokens the bucket can hold
func NewTokenBucket(rate, capacity int) *TokenBucket {
	return &TokenBucket{
		tokens:         float64(capacity),
		rate:           float64(rate),
		capacity:       float64(capacity),
		lastRefillTime: time.Now(),
	}
}

// Allow attempts to consume one token from the bucket.
// Returns true if a token was available (request allowed), false otherwise.
// This method is thread-safe and performs no network I/O.
func (tb *TokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Calculate refill amount since last access, capped at capacity
	now := time.Now()
	elapsed := now.Sub(tb.lastRefillTime).Seconds()
	refill := elapsed * tb.rate
	tb.tokens = minFloat(tb.capacity, tb.tokens+refill)
	tb.lastRefillTime = now

	// Consume one token if available
	if tb.tokens >= 1.0 {
		tb.tokens -= 1.0
		return true
	}
	return false
}

// Remaining returns the current number of tokens in the bucket.
func (tb *TokenBucket) Remaining() float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.tokens
}

// AddTokens adds tokens to the bucket (used by Redis prefetch sync).
func (tb *TokenBucket) AddTokens(count float64) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.tokens = minFloat(tb.capacity, tb.tokens+count)
}

// Rate returns the refill rate.
func (tb *TokenBucket) Rate() float64 { return tb.rate }

// Capacity returns the bucket capacity.
func (tb *TokenBucket) Capacity() float64 { return tb.capacity }

// LeakyBucket implements a thread-safe in-memory leaky bucket algorithm.
type LeakyBucket struct {
	mu            sync.Mutex
	level         float64   // current water level
	rate          float64   // leak rate (requests per second)
	capacity      float64   // maximum capacity
	lastLeakTime  time.Time // last leak timestamp
}

// NewLeakyBucket creates a new leaky bucket.
func NewLeakyBucket(rate, capacity int) *LeakyBucket {
	return &LeakyBucket{
		rate:         float64(rate),
		capacity:     float64(capacity),
		lastLeakTime: time.Now(),
	}
}

// Allow attempts to add one request to the bucket.
// Returns true if the bucket has space (request allowed), false otherwise.
func (lb *LeakyBucket) Allow() bool {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Leak water since last access
	now := time.Now()
	elapsed := now.Sub(lb.lastLeakTime).Seconds()
	leaked := elapsed * lb.rate
	lb.level = maxFloat(0, lb.level-leaked)
	lb.lastLeakTime = now

	// Add request if there is space
	if lb.level < lb.capacity {
		lb.level += 1.0
		return true
	}
	return false
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// RateLimiter manages multiple rate limiters keyed by rule ID + dimension value.
type RateLimiter struct {
	mu       sync.RWMutex
	buckets  map[string]*TokenBucket
	rate     int
	capacity int
}

// NewRateLimiter creates a new rate limiter manager.
func NewRateLimiter(rate, capacity int) *RateLimiter {
	return &RateLimiter{
		buckets:  make(map[string]*TokenBucket),
		rate:     rate,
		capacity: capacity,
	}
}

// Allow checks if the request identified by key should be allowed.
func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.RLock()
	bucket, ok := rl.buckets[key]
	rl.mu.RUnlock()

	if !ok {
		rl.mu.Lock()
		// Double-check after acquiring write lock
		if bucket, ok = rl.buckets[key]; !ok {
			bucket = NewTokenBucket(rl.rate, rl.capacity)
			rl.buckets[key] = bucket
		}
		rl.mu.Unlock()
	}
	return bucket.Allow()
}

// GetBucket returns the token bucket for the given key (for metrics/headers).
func (rl *RateLimiter) GetBucket(key string) *TokenBucket {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return rl.buckets[key]
}
