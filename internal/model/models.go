// Package model defines all GORM database models for the gateway configuration store.
//
// All tables use the gw_ prefix, UUID primary keys, and soft delete via deleted_at.
// The BaseModel struct embeds common fields used across all configuration tables.
package model

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// BaseModel embeds common fields for all configuration tables.
// Uses UUID primary key, soft delete (deleted_at), and optimistic locking (version).
type BaseModel struct {
	ID        string         `gorm:"type:varchar(36);primaryKey" json:"id"`
	CreatedAt time.Time      `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime" json:"updated_at"`
	CreatedBy string         `gorm:"type:varchar(64);not null;default:system" json:"created_by"`
	UpdatedBy string         `gorm:"type:varchar(64);not null;default:system" json:"updated_by"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
	Version   int            `gorm:"not null;default:0" json:"version"`
}

// Route represents a gateway routing rule (gw_route table).
type Route struct {
	BaseModel
	RouteName         string `gorm:"type:varchar(128);not null" json:"route_name"`
	Path              string `gorm:"type:varchar(512);not null;uniqueIndex:uk_route_path_method,priority:1" json:"path"`
	Method            string `gorm:"type:varchar(16);not null;uniqueIndex:uk_route_path_method,priority:2" json:"method"`
	TargetService     string `gorm:"type:varchar(128);not null;index:idx_route_target_service" json:"target_service"`
	RewritePath       string `gorm:"type:varchar(512)" json:"rewrite_path,omitempty"`
	TimeoutMs         int    `gorm:"not null;default:5000" json:"timeout_ms"`
	RetryCount        int    `gorm:"not null;default:0" json:"retry_count"`
	RetryOnStatus     string `gorm:"type:varchar(256)" json:"retry_on_status,omitempty"`
	RetryNonIdempotent bool  `gorm:"not null;default:false" json:"retry_non_idempotent"`
	LoadBalance       string `gorm:"type:varchar(32);not null;default:weighted_round_robin" json:"load_balance"`
	HashHeader        string `gorm:"type:varchar(64)" json:"hash_header,omitempty"`
	Priority          int    `gorm:"not null;default:100" json:"priority"`
	Enabled           bool   `gorm:"not null;default:true;index:idx_route_enabled" json:"enabled"`
	Description       string `gorm:"type:varchar(512)" json:"description,omitempty"`
}

// TableName specifies the database table name for Route.
func (Route) TableName() string { return "gw_route" }

// RateLimit represents a rate limiting rule (gw_rate_limit table).
type RateLimit struct {
	BaseModel
	RuleName    string `gorm:"type:varchar(128);not null" json:"rule_name"`
	RouteID     string `gorm:"type:varchar(36);index:idx_rl_route" json:"route_id,omitempty"`
	PathPattern string `gorm:"type:varchar(512);index:idx_rl_path,priority:1" json:"path_pattern,omitempty"`
	Method      string `gorm:"type:varchar(16);default:*;index:idx_rl_path,priority:2" json:"method"`
	Algorithm   string `gorm:"type:varchar(32);not null;default:token_bucket" json:"algorithm"` // token_bucket / leaky_bucket
	Rate        int    `gorm:"not null" json:"rate"`     // tokens per second
	Capacity    int    `gorm:"not null" json:"capacity"` // bucket capacity
	LimitBy     string `gorm:"type:varchar(32);not null;default:route" json:"limit_by"` // route / ip / client_id
	HeaderKey   string `gorm:"type:varchar(64)" json:"header_key,omitempty"`
	Burst       int    `json:"burst,omitempty"`
	Enabled     bool   `gorm:"not null;default:true;index:idx_rl_enabled" json:"enabled"`
	Description string `gorm:"type:varchar(512)" json:"description,omitempty"`
}

// TableName specifies the database table name for RateLimit.
func (RateLimit) TableName() string { return "gw_rate_limit" }

// CircuitBreaker represents a circuit breaker policy (gw_circuit_breaker table).
type CircuitBreaker struct {
	BaseModel
	PolicyName             string  `gorm:"type:varchar(128);not null" json:"policy_name"`
	RouteID                string  `gorm:"type:varchar(36);index:idx_cb_route" json:"route_id,omitempty"`
	TargetService          string  `gorm:"type:varchar(128);index:idx_cb_service" json:"target_service,omitempty"`
	SlidingWindowSizeMs   int     `gorm:"not null;default:10000" json:"sliding_window_size_ms"`
	MinRequestCount        int     `gorm:"not null;default:20" json:"min_request_count"`
	ErrorRateThreshold     float64 `gorm:"type:decimal(5,2);not null;default:50.00" json:"error_rate_threshold"`
	SlowCallDurationMs     int     `gorm:"not null;default:1000" json:"slow_call_duration_ms"`
	SlowCallRateThreshold  float64 `gorm:"type:decimal(5,2);not null;default:60.00" json:"slow_call_rate_threshold"`
	OpenDurationMs         int     `gorm:"not null;default:30000" json:"open_duration_ms"`
	HalfOpenMaxRequests    int     `gorm:"not null;default:5" json:"half_open_max_requests"`
	FailureStatusCodes     string  `gorm:"type:varchar(256);not null;default:500,502,503,504" json:"failure_status_codes"`
	Enabled                bool    `gorm:"not null;default:true;index:idx_cb_enabled" json:"enabled"`
	Description            string  `gorm:"type:varchar(512)" json:"description,omitempty"`
}

// TableName specifies the database table name for CircuitBreaker.
func (CircuitBreaker) TableName() string { return "gw_circuit_breaker" }

// GrayRelease represents a gray release rule (gw_gray_release table).
type GrayRelease struct {
	BaseModel
	RuleName     string         `gorm:"type:varchar(128);not null" json:"rule_name"`
	RouteID      string         `gorm:"type:varchar(36);not null;index:idx_gray_route" json:"route_id"`
	GrayVersion  string         `gorm:"type:varchar(64);not null" json:"gray_version"`
	StableVersion string        `gorm:"type:varchar(64);not null;default:stable" json:"stable_version"`
	GrayPercent  int            `gorm:"not null;default:0" json:"gray_percent"` // 0-100
	HashKeyType  string         `gorm:"type:varchar(32);not null;default:ip" json:"hash_key_type"` // ip/header/cookie/user_id
	HashKeyName  string         `gorm:"type:varchar(64)" json:"hash_key_name,omitempty"`
	StainRules   datatypes.JSON `gorm:"type:json" json:"stain_rules,omitempty"` // JSON array of stain rules
	Enabled      bool           `gorm:"not null;default:true;index:idx_gray_enabled" json:"enabled"`
	Description  string         `gorm:"type:varchar(512)" json:"description,omitempty"`
}

// TableName specifies the database table name for GrayRelease.
func (GrayRelease) TableName() string { return "gw_gray_release" }

// Service represents a backend service metadata entry (gw_service table).
type Service struct {
	BaseModel
	ServiceName        string `gorm:"type:varchar(128);not null;uniqueIndex:uk_service_name" json:"service_name"`
	DisplayName        string `gorm:"type:varchar(128)" json:"display_name,omitempty"`
	HealthCheckPath    string `gorm:"type:varchar(256);not null;default:/health" json:"health_check_path"`
	HealthCheckInterval int   `gorm:"not null;default:10" json:"health_check_interval"` // seconds
	HealthCheckTimeout  int   `gorm:"not null;default:3" json:"health_check_timeout"`   // seconds
	UnhealthyThreshold  int   `gorm:"not null;default:2" json:"unhealthy_threshold"`
	HealthyThreshold    int   `gorm:"not null;default:2" json:"healthy_threshold"`
	DefaultWeight       int   `gorm:"not null;default:1" json:"default_weight"`
	Description         string `gorm:"type:varchar(512)" json:"description,omitempty"`
}

// TableName specifies the database table name for Service.
func (Service) TableName() string { return "gw_service" }

// ConfigVersion records configuration change history with before/after snapshots.
type ConfigVersion struct {
	ID         int64          `gorm:"primaryKey;autoIncrement" json:"id"`
	ConfigType string         `gorm:"type:varchar(32);not null;index:idx_version_config,priority:1" json:"config_type"`
	ConfigID   string         `gorm:"type:varchar(36);not null;index:idx_version_config,priority:2" json:"config_id"`
	Action     string         `gorm:"type:varchar(16);not null" json:"action"` // create/update/delete
	Snapshot   datatypes.JSON `gorm:"type:json;not null" json:"snapshot"`       // {"before":{}, "after":{}}
	Operator   string         `gorm:"type:varchar(64);not null" json:"operator"`
	CreatedAt  time.Time      `gorm:"autoCreateTime;index:idx_version_created" json:"created_at"`
}

// TableName specifies the database table name for ConfigVersion.
func (ConfigVersion) TableName() string { return "gw_config_version" }

// OperationLog records admin-plane operation audit entries.
type OperationLog struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	TraceID       string    `gorm:"type:varchar(64)" json:"trace_id,omitempty"`
	Operator      string    `gorm:"type:varchar(64);not null;index:idx_oplog_operator" json:"operator"`
	Operation     string    `gorm:"type:varchar(128);not null" json:"operation"`
	ResourceType  string    `gorm:"type:varchar(32);not null;index:idx_oplog_resource,priority:1" json:"resource_type"`
	ResourceID    string    `gorm:"type:varchar(36);index:idx_oplog_resource,priority:2" json:"resource_id,omitempty"`
	RequestMethod string    `gorm:"type:varchar(16);not null" json:"request_method"`
	RequestPath   string    `gorm:"type:varchar(512);not null" json:"request_path"`
	RequestBody   string    `gorm:"type:text" json:"request_body,omitempty"` // sensitive fields masked
	ResponseCode  int       `gorm:"not null" json:"response_code"`
	ClientIP      string    `gorm:"type:varchar(64)" json:"client_ip,omitempty"`
	DurationMs    int       `gorm:"not null" json:"duration_ms"`
	CreatedAt     time.Time `gorm:"autoCreateTime;index:idx_oplog_created" json:"created_at"`
}

// TableName specifies the database table name for OperationLog.
func (OperationLog) TableName() string { return "gw_operation_log" }

// AllModels returns all GORM model types for AutoMigrate.
func AllModels() []interface{} {
	return []interface{}{
		&Route{},
		&RateLimit{},
		&CircuitBreaker{},
		&GrayRelease{},
		&Service{},
		&ConfigVersion{},
		&OperationLog{},
	}
}
