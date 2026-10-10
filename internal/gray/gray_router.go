// Package gray implements gray release routing calculation.
//
// Gray routing uses two-tier decision:
// 1. Stain rules (highest priority): Header/Cookie/Query exact match or UserID IN list
// 2. Hash-based proportion: murmur3(key) % 100 < gray_percent
//
// The result is a version label ("canary" or "stable") written to the request context.
package gray

import (
	"encoding/json"
	"hash/fnv"
	"net/http"
	"strings"
)

// StainRule defines a traffic staining rule for gray release.
type StainRule struct {
	Type     string `json:"type"`     // header / cookie / query / user_id
	Key      string `json:"key"`      // header/cookie/query name
	Value    string `json:"value"`    // exact match value
	UserIDs  []string `json:"user_ids,omitempty"` // for user_id type
}

// Rule represents a complete gray release rule.
type Rule struct {
	RouteID       string
	GrayVersion   string
	StableVersion string
	GrayPercent   int    // 0-100
	HashKeyType   string // ip / header / cookie / user_id
	HashKeyName   string
	StainRules    []StainRule
}

// Router calculates gray release version for incoming requests.
type Router struct{}

// NewRouter creates a new gray router.
func NewRouter() *Router {
	return &Router{}
}

// Calculate determines whether a request should go to the gray or stable version.
//
// Parameters:
//   - rule: the gray release rule
//   - r: the incoming HTTP request (for header/cookie/query extraction)
//   - clientIP: the client IP address
//
// Returns:
//   - version: "canary" (gray) or "stable"
//   - matched: whether a stain rule matched (always true for proportion-based)
func (gr *Router) Calculate(rule *Rule, r *http.Request, clientIP string) (version string, matched bool) {
	if rule == nil {
		return "stable", false
	}

	// Tier 1: Stain rules (highest priority)
	for _, stain := range rule.StainRules {
		if gr.matchStain(stain, r, clientIP) {
			return rule.GrayVersion, true
		}
	}

	// Tier 2: Hash-based proportion
	if rule.GrayPercent <= 0 {
		return rule.StableVersion, false
	}
	if rule.GrayPercent >= 100 {
		return rule.GrayVersion, true
	}

	hashKey := gr.extractHashKey(rule, r, clientIP)
	hashValue := hashUint32([]byte(hashKey))
	if int(hashValue%100) < rule.GrayPercent {
		return rule.GrayVersion, true
	}
	return rule.StableVersion, false
}

// matchStain checks if a single stain rule matches the request.
func (gr *Router) matchStain(rule StainRule, r *http.Request, clientIP string) bool {
	switch rule.Type {
	case "header":
		return r.Header.Get(rule.Key) == rule.Value
	case "cookie":
		cookie, err := r.Cookie(rule.Key)
		if err != nil {
			return false
		}
		return cookie.Value == rule.Value
	case "query":
		return r.URL.Query().Get(rule.Key) == rule.Value
	case "user_id":
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			userID = r.URL.Query().Get("user_id")
		}
		for _, id := range rule.UserIDs {
			if id == userID {
				return true
			}
		}
		return false
	case "ip":
		return clientIP == rule.Value
	}
	return false
}

// extractHashKey extracts the key used for hash-based proportion routing.
func (gr *Router) extractHashKey(rule *Rule, r *http.Request, clientIP string) string {
	switch rule.HashKeyType {
	case "header":
		if rule.HashKeyName != "" {
			return r.Header.Get(rule.HashKeyName)
		}
	case "cookie":
		if rule.HashKeyName != "" {
			cookie, err := r.Cookie(rule.HashKeyName)
			if err == nil {
				return cookie.Value
			}
		}
	case "user_id":
		userID := r.Header.Get("X-User-ID")
		if userID == "" {
			userID = r.URL.Query().Get("user_id")
		}
		return userID
	}
	// Default: client IP
	return clientIP
}

// ParseStainRules parses a JSON string into a slice of StainRule.
func ParseStainRules(data []byte) ([]StainRule, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var rules []StainRule
	if err := json.Unmarshal(data, &rules); err != nil {
		return nil, err
	}
	return rules, nil
}

func hashUint32(data []byte) uint32 {
	h := fnv.New32a()
	h.Write(data)
	return h.Sum32()
}

// NormalizeVersion ensures version string is non-empty.
func NormalizeVersion(v string) string {
	if strings.TrimSpace(v) == "" {
		return "stable"
	}
	return v
}
