package ratelimit

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/go-api-gateway/gateway/internal/config"
	"github.com/go-api-gateway/gateway/internal/model"
)

// ruleLimiter wraps a token/leaky bucket with its rule metadata.
type ruleLimiter struct {
	rule    *model.RateLimit
	mu      sync.Mutex
	buckets map[string]bucket // 修复Bug-P1-001 key: 维度key(global/ip:xxx/client:xxx)，按LimitBy维度分桶
}

// bucket is the common interface for TokenBucket and LeakyBucket.
type bucket interface {
	Allow() bool
}

// RedisClient is the interface for Redis operations needed by distributed rate limiting.
// 修复Bug-P1-007 定义Redis操作接口，由store.RedisClient实现，避免循环依赖
type RedisClient interface {
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
}

// RateLimiterManager manages multiple rate limiter instances keyed by rule ID.
// It is synchronized with the config snapshot via Sync() on hot reload.
type RateLimiterManager struct {
	mu          sync.RWMutex
	limiters    map[string]*ruleLimiter // key: rule ID
	redisClient RedisClient             // 修复Bug-P1-007 Redis分布式限流客户端，为nil时退化为纯本地模式
	rlCfg       config.RateLimitConfig  // 修复Bug-P1-007 限流配置(预取间隔、预取因子等)
	stopCh      chan struct{}           // 修复Bug-P1-007 后台预取goroutine停止信号
}

// NewRateLimiterManager creates a new empty manager.
func NewRateLimiterManager() *RateLimiterManager {
	return &RateLimiterManager{
		limiters: make(map[string]*ruleLimiter),
	}
}

// SetRedisClient injects the Redis client for distributed rate limiting and starts
// the background token prefetch goroutine. 修复Bug-P1-007
func (m *RateLimiterManager) SetRedisClient(rc RedisClient, cfg config.RateLimitConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.redisClient = rc
	m.rlCfg = cfg
	if rc != nil && m.stopCh == nil {
		m.stopCh = make(chan struct{})
		go m.prefetchLoop()
	}
}

// Stop stops the background prefetch goroutine. 修复Bug-P1-007
func (m *RateLimiterManager) Stop() {
	m.mu.RLock()
	ch := m.stopCh
	m.mu.RUnlock()
	if ch != nil {
		close(ch)
	}
}

// prefetchLoop periodically fetches tokens from Redis to refill local buckets.
// 修复Bug-P1-007 两层限流架构：本地内存快速路径 + Redis批量预取
func (m *RateLimiterManager) prefetchLoop() {
	interval := m.rlCfg.PrefetchInterval
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.prefetchFromRedis()
		case <-m.stopCh:
			return
		}
	}
}

// prefetchFromRedis fetches tokens from Redis for all active limiters and refills local buckets.
// 修复Bug-P1-007 每个规则+维度桶独立从Redis预取令牌
func (m *RateLimiterManager) prefetchFromRedis() {
	m.mu.RLock()
	rc := m.redisClient
	limiters := make([]*ruleLimiter, 0, len(m.limiters))
	for _, rl := range m.limiters {
		limiters = append(limiters, rl)
	}
	m.mu.RUnlock()
	if rc == nil {
		return
	}

	factor := m.rlCfg.PrefetchFactor
	if factor <= 0 {
		factor = 2
	}

	for _, rl := range limiters {
		rl.mu.Lock()
		for dimKey, bkt := range rl.buckets {
			tb, ok := bkt.(*TokenBucket)
			if !ok {
				continue
			}
			// 计算需要预取的令牌数：本地桶剩余不足时从Redis补充
			needed := float64(rl.rule.Rate*factor) - tb.Remaining()
			if needed <= 0 {
				continue
			}
			redisKey := "gw:ratelimit:" + rl.rule.ID + ":" + dimKey
			granted, err := fetchRedisTokens(rc, redisKey, rl.rule.Rate, rl.rule.Capacity, needed)
			if err == nil && granted > 0 {
				tb.AddTokens(granted)
			}
		}
		rl.mu.Unlock()
	}
}

// Sync reconciles internal limiters with the latest snapshot rules.
// - New rules: create limiter instances
// - Changed rules: replace with new config (resets bucket state)
// - Removed rules: delete instances
func (m *RateLimiterManager) Sync(rules []*model.RateLimit) {
	m.mu.Lock()
	defer m.mu.Unlock()

	active := make(map[string]bool, len(rules))
	for _, rule := range rules {
		active[rule.ID] = true
		existing, ok := m.limiters[rule.ID]
		// Create new or replace if config changed (compare key fields)
		if !ok || !ruleEqual(existing.rule, rule) {
			m.limiters[rule.ID] = &ruleLimiter{
				rule:    rule,
				buckets: make(map[string]bucket),
			}
		}
	}
	// Remove rules no longer in snapshot
	for id := range m.limiters {
		if !active[id] {
			delete(m.limiters, id)
		}
	}
}

// Get returns the limiter for the given rule ID, or nil if not found.
func (m *RateLimiterManager) Get(ruleID string) *ruleLimiter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.limiters[ruleID]
}

// Allow checks all limiters for the given rules. Returns the first rejecting
// rule (so caller can set accurate headers), or nil if all pass.
// 修复Bug-P1-001 新增clientIP和headers参数，根据rule.LimitBy维度生成不同key分桶限流
func (m *RateLimiterManager) Allow(rules []*model.RateLimit, clientIP string, headers http.Header) *model.RateLimit {
	for _, rule := range rules {
		rl := m.Get(rule.ID)
		if rl == nil {
			continue
		}
		// 修复Bug-P1-001 根据LimitBy维度生成桶key
		dimKey := buildLimitKey(rule, clientIP, headers)
		rl.mu.Lock()
		bkt, ok := rl.buckets[dimKey]
		if !ok {
			bkt = newBucket(rule)
			rl.buckets[dimKey] = bkt
		}
		rl.mu.Unlock()
		if !bkt.Allow() {
			return rule
		}
	}
	return nil
}

// buildLimitKey generates the dimension key for rate limiting bucket.
// 修复Bug-P1-001 route=全局共享, ip=按客户端IP, client_id=按指定Header值
func buildLimitKey(rule *model.RateLimit, clientIP string, headers http.Header) string {
	switch rule.LimitBy {
	case "ip":
		return "ip:" + clientIP
	case "client_id":
		if rule.HeaderKey != "" {
			val := headers.Get(rule.HeaderKey)
			if val != "" {
				return "client:" + val
			}
		}
		return "client:" + clientIP
	default: // route
		return "global"
	}
}

// Rule returns the rule metadata for a limiter.
func (rl *ruleLimiter) Rule() *model.RateLimit { return rl.rule }

func newBucket(rule *model.RateLimit) bucket {
	if rule.Algorithm == "leaky_bucket" {
		return NewLeakyBucket(rule.Rate, rule.Capacity)
	}
	return NewTokenBucket(rule.Rate, rule.Capacity)
}

func ruleEqual(a, b *model.RateLimit) bool {
	return a.Algorithm == b.Algorithm &&
		a.Rate == b.Rate &&
		a.Capacity == b.Capacity &&
		a.LimitBy == b.LimitBy &&
		a.Enabled == b.Enabled
}
