package ratelimit

import (
	"context"
	"fmt"
	"time"
)

// redisTokenBucketScript is a Lua script that implements an atomic token bucket in Redis.
// KEYS[1] = bucket key
// ARGV[1] = refill rate (tokens per second)
// ARGV[2] = bucket capacity
// ARGV[3] = requested tokens
// ARGV[4] = current timestamp (milliseconds)
// Returns the number of tokens granted (may be less than requested if bucket is empty).
// 修复Bug-P1-007 Redis分布式限流核心：原子令牌桶Lua脚本
const redisTokenBucketScript = `
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local requested = tonumber(ARGV[3])
local now = tonumber(ARGV[4])

local bucket = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(bucket[1])
local last_refill = tonumber(bucket[2])

if tokens == nil then
    tokens = capacity
    last_refill = now
end

local elapsed = (now - last_refill) / 1000.0
tokens = math.min(capacity, tokens + elapsed * rate)

local granted = 0.0
if tokens >= requested then
    granted = requested
    tokens = tokens - requested
else
    granted = tokens
    tokens = 0
end

redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
redis.call('EXPIRE', key, 60)
return granted
`

// fetchRedisTokens attempts to fetch the requested number of tokens from the Redis token bucket.
// Returns the number of tokens actually granted (0 if Redis is unavailable or bucket empty).
// 修复Bug-P1-007 从Redis分布式令牌桶预取令牌，多实例共享全局配额
func fetchRedisTokens(rc RedisClient, key string, rate, capacity int, requested float64) (float64, error) {
	if rc == nil {
		return 0, fmt.Errorf("redis client is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	now := time.Now().UnixMilli()
	result, err := rc.Eval(ctx, redisTokenBucketScript,
		[]string{key},
		rate, capacity, requested, now,
	)
	if err != nil {
		return 0, err
	}

	// Lua returns integer; convert to float64
	switch v := result.(type) {
	case int64:
		return float64(v), nil
	case float64:
		return v, nil
	default:
		return 0, fmt.Errorf("unexpected redis response type: %T", result)
	}
}
