// Package utils provides common utility functions used across the gateway.
package utils

import (
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// NewUUID generates a new UUID v4 string.
func NewUUID() string {
	return uuid.NewString()
}

// GenerateTraceID generates a 32-character hex trace ID (W3C TraceContext format).
func GenerateTraceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// GenerateSpanID generates a 16-character hex span ID.
func GenerateSpanID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// ClientIP extracts the client IP from the request.
// It checks X-Forwarded-For (multi-level), X-Real-IP, then RemoteAddr.
func ClientIP(r *http.Request) string {
	// X-Forwarded-For may contain multiple IPs: client, proxy1, proxy2
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// NowUTC returns the current time in UTC.
func NowUTC() time.Time {
	return time.Now().UTC()
}

// FormatTime formats a time.Time as ISO 8601 UTC string.
func FormatTime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// ContainsString checks if a string slice contains the given string.
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// ParseStatusCodes parses a comma-separated list of HTTP status codes.
func ParseStatusCodes(s string) []int {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	codes := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		var code int
		for _, c := range p {
			if c >= '0' && c <= '9' {
				code = code*10 + int(c-'0')
			}
		}
		if code > 0 {
			codes = append(codes, code)
		}
	}
	return codes
}

// Min returns the smaller of two integers.
func Min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Max returns the larger of two integers.
func Max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
