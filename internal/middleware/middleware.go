// Package middleware provides Gin middleware for the gateway request pipeline.
//
// Data-plane middleware order: Trace -> RateLimit -> CircuitBreaker -> Gray -> RouteMatch -> Proxy
// Admin-plane middleware: Auth -> Audit -> Recovery
package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-api-gateway/gateway/internal/circuitbreaker"
	"github.com/go-api-gateway/gateway/internal/configsnap"
	"github.com/go-api-gateway/gateway/internal/gray"
	"github.com/go-api-gateway/gateway/internal/pkg/errcode"
	"github.com/go-api-gateway/gateway/internal/pkg/response"
	"github.com/go-api-gateway/gateway/internal/pkg/utils"
	"github.com/go-api-gateway/gateway/internal/ratelimit"
)

// TraceMiddleware extracts or generates a TraceId and injects it into the context.
// 100% of requests get a TraceId (baseline T-01).
func TraceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		traceID := c.GetHeader("traceparent")
		if traceID == "" {
			traceID = utils.GenerateTraceID()
		} else {
			// Extract trace ID from W3C traceparent format: 00-{traceid}-{spanid}-{flags}
			parts := splitTraceParent(traceID)
			if len(parts) >= 2 {
				traceID = parts[1]
			}
		}
		c.Set("trace_id", traceID)
		c.Set("start_time", time.Now())
		c.Header("X-Trace-Id", traceID)
		c.Next()
	}
}

func splitTraceParent(s string) []string {
	var parts []string
	current := ""
	for _, c := range s {
		if c == '-' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	parts = append(parts, current)
	return parts
}

// RateLimitMiddleware checks rate limits for the matched route.
// Returns 429 with X-RateLimit-* headers if rate limited.
func RateLimitMiddleware(rl *ratelimit.RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		routeID := c.GetString("route_id")
		if routeID == "" {
			c.Next()
			return
		}

		if !rl.Allow(routeID) {
			c.Header("X-RateLimit-Limit", "100")
			c.Header("X-RateLimit-Remaining", "0")
			c.Header("Retry-After", "1")
			response.Fail(c, http.StatusTooManyRequests, errcode.TooManyRequests, "rate limit exceeded")
			c.Abort()
			return
		}
		c.Next()
	}
}

// CircuitBreakerMiddleware checks the circuit breaker state for the target service.
// Open state returns 503 with X-Circuit-Breaker header.
func CircuitBreakerMiddleware(cbManager *circuitbreaker.CircuitBreaker) gin.HandlerFunc {
	return func(c *gin.Context) {
		if cbManager != nil && !cbManager.Allow() {
			c.Header("X-Circuit-Breaker", "open")
			response.Fail(c, http.StatusServiceUnavailable, 50300, "circuit breaker open")
			c.Abort()
			return
		}
		c.Next()
	}
}

// GrayMiddleware determines gray/stable version for the request.
func GrayMiddleware(grayRouter *gray.Router, snap *configsnap.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		routeID := c.GetString("route_id")
		if routeID == "" {
			c.Set("gray_version", "stable")
			c.Next()
			return
		}

		snapshot := snap.Get()
		grayRule := snapshot.GetGrayRuleByRouteID(routeID)
		if grayRule == nil {
			c.Set("gray_version", "stable")
			c.Next()
			return
		}

		stainRules, _ := gray.ParseStainRules(grayRule.StainRules)
		rule := &gray.Rule{
			RouteID:       grayRule.RouteID,
			GrayVersion:   grayRule.GrayVersion,
			StableVersion: grayRule.StableVersion,
			GrayPercent:   grayRule.GrayPercent,
			HashKeyType:   grayRule.HashKeyType,
			HashKeyName:   grayRule.HashKeyName,
			StainRules:    stainRules,
		}

		clientIP := utils.ClientIP(c.Request)
		version, _ := grayRouter.Calculate(rule, c.Request, clientIP)
		c.Set("gray_version", version)
		c.Next()
	}
}

// RecoveryMiddleware recovers from panics and returns a 500 response.
func RecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				response.Fail(c, http.StatusInternalServerError, errcode.InternalError, "internal server error")
				c.Abort()
			}
		}()
		c.Next()
	}
}

// AuthMiddleware validates admin-plane Bearer token authentication.
// Supports multi-token with username mapping (P2-6).
func AuthMiddleware(tokens map[string]string, defaultUsername string) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || len(auth) < 7 || auth[:7] != "Bearer " {
			response.Fail(c, http.StatusUnauthorized, errcode.Unauthorized, "missing or invalid authorization token")
			c.Abort()
			return
		}
		token := auth[7:]
		username, ok := tokens[token]
		if !ok {
			response.Fail(c, http.StatusUnauthorized, errcode.Unauthorized, "invalid token")
			c.Abort()
			return
		}
		// Operator resolution: token mapping > X-Operator header > default
		operator := c.GetHeader("X-Operator")
		if operator == "" {
			operator = username
		}
		if operator == "" {
			operator = defaultUsername
		}
		c.Set("operator", operator)
		c.Next()
	}
}

// CORSMiddleware adds CORS headers for admin API access.
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Operator")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
