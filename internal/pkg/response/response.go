// Package response provides the unified API response structure and helper functions.
//
// All admin-plane API responses follow the format:
//
//	{ "code": 0, "message": "success", "data": {...}, "trace_id": "..." }
//
// Paginated responses include a pagination object inside data.
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response is the unified API response envelope.
type Response struct {
	Code    int         `json:"code"`              // 0=success, non-zero=error code
	Message string      `json:"message"`           // human-readable message
	Data    interface{} `json:"data,omitempty"`    // response payload
	TraceID string      `json:"trace_id,omitempty"` // distributed trace ID
	Errors  []FieldError `json:"errors,omitempty"`  // field-level validation errors
}

// FieldError describes a validation error for a specific field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Pagination holds pagination metadata for list responses.
type Pagination struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// PageData is the data wrapper for paginated responses.
type PageData struct {
	List       interface{} `json:"list"`
	Pagination Pagination  `json:"pagination"`
}

// Success writes a successful response with the given data.
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
		TraceID: getTraceID(c),
	})
}

// PageSuccess writes a paginated successful response.
func PageSuccess(c *gin.Context, list interface{}, page, pageSize int, total int64) {
	totalPages := 0
	if pageSize > 0 {
		totalPages = int((total + int64(pageSize) - 1) / int64(pageSize))
	}
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data: PageData{
			List: list,
			Pagination: Pagination{
				Page:       page,
				PageSize:   pageSize,
				Total:      total,
				TotalPages: totalPages,
			},
		},
		TraceID: getTraceID(c),
	})
}

// Fail writes an error response with the given error code and message.
func Fail(c *gin.Context, httpStatus, code int, message string) {
	c.JSON(httpStatus, Response{
		Code:    code,
		Message: message,
		TraceID: getTraceID(c),
	})
}

// FailWithErrors writes an error response with field-level validation errors.
func FailWithErrors(c *gin.Context, httpStatus, code int, message string, errs []FieldError) {
	c.JSON(httpStatus, Response{
		Code:    code,
		Message: message,
		Errors:  errs,
		TraceID: getTraceID(c),
	})
}

// getTraceID extracts the trace ID from the gin context if set by the trace middleware.
func getTraceID(c *gin.Context) string {
	if v, exists := c.Get("trace_id"); exists {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
