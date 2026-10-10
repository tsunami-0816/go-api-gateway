// Package config provides configuration loading and management for the gateway.
//
// It supports YAML config files with environment variable overrides (GATEWAY_ prefix),
// and defines all configuration sections used across the application.
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the root configuration struct holding all gateway settings.
type Config struct {
	Server         ServerConfig             `mapstructure:"server"`
	ConfigStore    ConfigStoreConfig        `mapstructure:"config_store"`
	Etcd           EtcdConfig               `mapstructure:"etcd"`
	Redis          RedisConfig              `mapstructure:"redis"`
	Admin          AdminConfig              `mapstructure:"admin"`
	Log            LogConfig                `mapstructure:"log"`
	Trace          TraceConfig              `mapstructure:"trace"`
	RateLimit      RateLimitConfig          `mapstructure:"ratelimit"`
	CircuitBreaker CircuitBreakerConfig     `mapstructure:"circuitbreaker"`
	Health         HealthConfig             `mapstructure:"health"`
	StaticBackends map[string][]StaticBackend `mapstructure:"static_backends"` // 修复Bug-P1-008 静态后端降级配置
}

// ServerConfig holds data-plane and admin-plane HTTP server settings.
type ServerConfig struct {
	Data  DataServerConfig  `mapstructure:"data"`
	Admin AdminServerConfig `mapstructure:"admin"`
}

// DataServerConfig is the data-plane (business traffic) server config.
type DataServerConfig struct {
	Host           string        `mapstructure:"host"`
	Port           int           `mapstructure:"port"`
	ReadTimeout    time.Duration `mapstructure:"read_timeout"`
	WriteTimeout   time.Duration `mapstructure:"write_timeout"`
	IdleTimeout    time.Duration `mapstructure:"idle_timeout"`
	MaxHeaderBytes int           `mapstructure:"max_header_bytes"`
}

// AdminServerConfig is the admin-plane (management API) server config.
type AdminServerConfig struct {
	Host string `mapstructure:"host"`
	Port int    `mapstructure:"port"`
}

// ConfigStoreConfig holds the configuration storage (SQLite/MySQL) settings.
type ConfigStoreConfig struct {
	Driver          string        `mapstructure:"driver"`
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// EtcdConfig holds etcd client connection settings.
type EtcdConfig struct {
	Endpoints         []string      `mapstructure:"endpoints"`
	DialTimeout       time.Duration `mapstructure:"dial_timeout"`
	AutoSyncInterval  time.Duration `mapstructure:"auto_sync_interval"`
}

// RedisConfig holds Redis client connection and pool settings.
type RedisConfig struct {
	Addr         string        `mapstructure:"addr"`
	Password     string        `mapstructure:"password"`
	DB           int           `mapstructure:"db"`
	PoolSize     int           `mapstructure:"pool_size"`
	MinIdleConns int           `mapstructure:"min_idle_conns"`
	MaxIdleConns int           `mapstructure:"max_idle_conns"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

// AdminConfig holds admin-plane authentication settings.
type AdminConfig struct {
	Auth AdminAuthConfig `mapstructure:"auth"`
}

// AdminAuthConfig defines token-based authentication with username mapping (P2-6).
type AdminAuthConfig struct {
	Enabled         bool          `mapstructure:"enabled"`
	DefaultUsername string        `mapstructure:"default_username"`
	Tokens          []TokenEntry  `mapstructure:"tokens"`
}

// TokenEntry maps a static token to a username for operation audit.
type TokenEntry struct {
	Token    string `mapstructure:"token"`
	Username string `mapstructure:"username"`
}

// LogConfig holds zap logger settings.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
	Async  bool   `mapstructure:"async"`
}

// TraceConfig holds OpenTelemetry tracing settings.
type TraceConfig struct {
	Enabled      bool    `mapstructure:"enabled"`
	Sampler      string  `mapstructure:"sampler"`
	SampleRatio  float64 `mapstructure:"sample_ratio"`
	Exporter     string  `mapstructure:"exporter"`
	OTLPEndpoint string  `mapstructure:"otlp_endpoint"`
}

// RateLimitConfig holds the two-layer rate limiter settings.
type RateLimitConfig struct {
	PrefetchInterval    time.Duration `mapstructure:"prefetch_interval"`
	PrefetchFactor      int           `mapstructure:"prefetch_factor"`
	GlobalRefillInterval time.Duration `mapstructure:"global_refill_interval"`
	KeyTTL              time.Duration `mapstructure:"key_ttl"`
}

// CircuitBreakerConfig holds circuit breaker global settings.
type CircuitBreakerConfig struct {
	EventChannel string `mapstructure:"event_channel"`
}

// StaticBackend represents a statically configured backend instance for fallback when etcd is unavailable.
// 修复Bug-P1-008 etcd连接失败时，网关可通过静态配置继续转发到指定后端
type StaticBackend struct {
	Address string `mapstructure:"address"`
	Weight  int    `mapstructure:"weight"`
}

// HealthConfig holds health check probe settings.
type HealthConfig struct {
	StartupDuration time.Duration `mapstructure:"startup_duration"`
	CheckInterval   time.Duration `mapstructure:"check_interval"`
}

// ShutdownTimeout returns the graceful shutdown timeout (30s per architecture).
func (c *Config) ShutdownTimeout() time.Duration {
	return 30 * time.Second
}

// Load reads configuration from the given YAML file path and applies
// environment variable overrides with the GATEWAY_ prefix.
//
// Parameters:
//   - path: path to the YAML config file
//
// Returns:
//   - *Config: loaded and validated configuration
//   - error: file read or parse error
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// Environment variable override: GATEWAY_ prefix, nested keys use underscore
	v.SetEnvPrefix("GATEWAY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	// Apply defaults for zero values
	applyDefaults(&cfg)
	return &cfg, nil
}

// applyDefaults fills in sensible defaults for optional config fields.
func applyDefaults(cfg *Config) {
	if cfg.Server.Data.Port == 0 {
		cfg.Server.Data.Port = 8080
	}
	if cfg.Server.Admin.Port == 0 {
		cfg.Server.Admin.Port = 8081
	}
	if cfg.Server.Data.Host == "" {
		cfg.Server.Data.Host = "0.0.0.0"
	}
	if cfg.Server.Admin.Host == "" {
		cfg.Server.Admin.Host = "0.0.0.0"
	}
	if cfg.ConfigStore.Driver == "" {
		cfg.ConfigStore.Driver = "sqlite"
	}
	if cfg.ConfigStore.MaxOpenConns == 0 {
		cfg.ConfigStore.MaxOpenConns = 100
	}
	if cfg.ConfigStore.MaxIdleConns == 0 {
		cfg.ConfigStore.MaxIdleConns = 20
	}
	if cfg.RateLimit.PrefetchInterval == 0 {
		cfg.RateLimit.PrefetchInterval = 100 * time.Millisecond
	}
	if cfg.RateLimit.PrefetchFactor == 0 {
		cfg.RateLimit.PrefetchFactor = 2
	}
	if cfg.Health.StartupDuration == 0 {
		cfg.Health.StartupDuration = 60 * time.Second
	}
}
