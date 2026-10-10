// Package errcode defines all API error codes following the 4xxyyy convention.
//
// Code format:
//
//	4  01  01
//	│  │   └─ error sequence (01-99)
//	│  └───── module (00=general, 01=route, 02=rate_limit, 03=circuit_breaker,
//	│            04=gray_release, 05=service, 09=system)
//	└──────── HTTP class (4=client, 5=server)
package errcode

// General error codes (module 00)
const (
	Success            = 0
	BadRequest         = 40000 // 参数错误
	ValidationFailed   = 40001 // 校验失败
	Unauthorized       = 40100 // 未授权
	Forbidden          = 40300 // 禁止访问
	NotFound           = 40400 // 资源不存在
	Conflict           = 40900 // 冲突
	TooManyRequests    = 42900 // 请求过于频繁
	InternalError      = 50000 // 内部错误
	DBError            = 50001 // 数据库操作失败
	ConfigReloadFailed = 50002 // 配置热更新失败
	DependencyUnavailable = 50300 // 依赖不可用
)

// Route module error codes (module 01)
const (
	RoutePathMethodExists = 40101 // 路由路径+方法已存在
	RouteTargetNotFound   = 40102 // 目标服务不存在
	RouteNotFound         = 40103 // 路由不存在
)

// Rate limit module error codes (module 02)
const (
	RateLimitAlgorithmUnsupported = 40201 // 限流算法不支持
	RateLimitRateMustBePositive   = 40202 // rate 必须大于0
)

// Circuit breaker module error codes (module 03)
const (
	CircuitBreakerErrorRateInvalid = 40301 // 错误率阈值必须在1-100之间
)

// Gray release module error codes (module 04)
const (
	GrayReleasePercentInvalid = 40401 // 灰度比例必须在0-100之间
)

// Service module error codes (module 05)
const (
	ServiceNameExists = 40501 // 服务名已存在
)

// ErrorMessage maps error codes to human-readable Chinese messages.
var ErrorMessage = map[int]string{
	Success:            "success",
	BadRequest:         "请求参数错误",
	ValidationFailed:   "参数校验失败",
	Unauthorized:       "未授权或Token无效",
	Forbidden:          "禁止访问",
	NotFound:           "资源不存在",
	Conflict:           "资源冲突",
	TooManyRequests:    "请求过于频繁",
	InternalError:      "服务器内部错误",
	DBError:            "数据库操作失败",
	ConfigReloadFailed: "配置热更新失败",
	DependencyUnavailable: "依赖服务不可用",

	RoutePathMethodExists: "路由路径与方法组合已存在",
	RouteTargetNotFound:   "目标服务不存在",
	RouteNotFound:         "路由不存在",

	RateLimitAlgorithmUnsupported: "不支持的限流算法",
	RateLimitRateMustBePositive:   "限流速率必须大于0",

	CircuitBreakerErrorRateInvalid: "错误率阈值必须在1-100之间",

	GrayReleasePercentInvalid: "灰度比例必须在0-100之间",

	ServiceNameExists: "服务名已存在",
}

// GetMessage returns the human-readable message for the given error code.
func GetMessage(code int) string {
	if msg, ok := ErrorMessage[code]; ok {
		return msg
	}
	return "未知错误"
}
