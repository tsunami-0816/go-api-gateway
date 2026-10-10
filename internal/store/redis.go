package store

import (
	"context"
	"time"

	"github.com/go-api-gateway/gateway/internal/config"
	"github.com/redis/go-redis/v9"
)

// RedisClient wraps the go-redis client with health check and helper methods.
type RedisClient struct {
	client *redis.Client
}

// NewRedisClient creates a new Redis client with connection pool configuration.
//
// Parameters:
//   - cfg: Redis configuration (addr, password, pool settings)
//
// Returns:
//   - *RedisClient: initialized Redis client
//   - error: connection failure
func NewRedisClient(cfg config.RedisConfig) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		MaxIdleConns: cfg.MaxIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &RedisClient{client: client}, nil
}

// Client returns the underlying go-redis client for direct operations.
func (r *RedisClient) Client() *redis.Client {
	return r.client
}

// Ping checks Redis connectivity.
func (r *RedisClient) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// Eval executes a Lua script with the given keys and arguments.
func (r *RedisClient) Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error) {
	return r.client.Eval(ctx, script, keys, args...).Result()
}

// Close closes the Redis client connection.
func (r *RedisClient) Close() error {
	return r.client.Close()
}
