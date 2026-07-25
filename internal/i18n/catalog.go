// Package i18n provides the small English and Simplified Chinese message catalog.
package i18n

import "strings"

// Language identifies one supported UI language.
type Language string

const (
	English           Language = "en"
	SimplifiedChinese Language = "zh-Hans"
)

// Catalog is an immutable localized message lookup.
type Catalog struct {
	language Language
	messages map[string]string
}

// New returns a catalog for the selected language.
func New(language Language) *Catalog {
	if language == SimplifiedChinese {
		return &Catalog{language: language, messages: chineseMessages}
	}
	return &Catalog{language: English, messages: englishMessages}
}

// System returns a catalog selected from the current OS locale.
func System() *Catalog {
	return New(languageFromLocale(detectLocale()))
}

// Language returns the selected catalog language.
func (c *Catalog) Language() Language {
	return c.language
}

// Message satisfies the proxy localizer interface.
func (c *Catalog) Message(code string) string {
	return c.Text(code)
}

// Text returns a localized stable key, falling back to the English catalog.
func (c *Catalog) Text(key string) string {
	if message, ok := c.messages[key]; ok {
		return message
	}
	if message, ok := englishMessages[key]; ok {
		return message
	}
	return key
}

func languageFromLocale(locale string) Language {
	normalized := strings.ToLower(strings.TrimSpace(locale))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	if normalized == "zh" || strings.HasPrefix(normalized, "zh-") {
		return SimplifiedChinese
	}
	return English
}

var englishMessages = map[string]string{
	"invalid_api_key":               "The local API key is missing or invalid.",
	"method_not_allowed":            "This HTTP method is not allowed for the local endpoint.",
	"service_unavailable":           "Kitsune Proxy is not ready to serve requests.",
	"unsupported_media_type":        "The request body must use an application/json media type.",
	"unsupported_content_encoding":  "The request Content-Encoding is not supported.",
	"request_body_too_large":        "The request body exceeds the configured safety limit.",
	"invalid_json":                  "The request body is not valid JSON.",
	"missing_model":                 "The request body must contain one top-level model string.",
	"invalid_model":                 "The top-level model value must be a string.",
	"duplicate_model":               "The top-level model field must appear exactly once.",
	"unknown_model":                 "The requested model is not configured.",
	"body_integrity_not_supported":  "Body integrity headers cannot be preserved while rewriting a model alias.",
	"websocket_not_supported":       "WebSocket forwarding is not supported.",
	"upstream_error":                "The configured upstream could not be reached.",
	"upstream_timeout":              "The configured upstream did not return response headers in time.",
	"menu_status_starting":          "Starting",
	"menu_status_running":           "Running on %s",
	"menu_status_config_error":      "Configuration error",
	"menu_status_config_error_at":   "Configuration error; still running on %s",
	"menu_status_listener_error":    "Listener error",
	"menu_status_listener_error_at": "Listener error at %s",
	"menu_status_stopped":           "Stopped",
	"menu_models":                   "Models: %d",
	"menu_logging_unavailable":      "Logging unavailable",
	"menu_open_config":              "Open Configuration",
	"menu_reload":                   "Reload Configuration",
	"menu_open_logs":                "Open Logs",
	"menu_autostart":                "Start at Login",
	"menu_quit":                     "Quit",
	"tooltip":                       "Kitsune Proxy",
}

var chineseMessages = map[string]string{
	"invalid_api_key":               "本地 API Key 缺失或无效。",
	"method_not_allowed":            "本地接口不支持此 HTTP 方法。",
	"service_unavailable":           "Kitsune Proxy 尚未准备好处理请求。",
	"unsupported_media_type":        "请求体必须使用 application/json 媒体类型。",
	"unsupported_content_encoding":  "不支持此请求 Content-Encoding。",
	"request_body_too_large":        "请求体超过安全大小限制。",
	"invalid_json":                  "请求体不是有效的 JSON。",
	"missing_model":                 "请求体必须包含唯一的顶层 model 字符串。",
	"invalid_model":                 "顶层 model 值必须是字符串。",
	"duplicate_model":               "顶层 model 字段只能出现一次。",
	"unknown_model":                 "请求的模型尚未配置。",
	"body_integrity_not_supported":  "改写模型别名时无法保留请求体完整性标头。",
	"websocket_not_supported":       "不支持 WebSocket 转发。",
	"upstream_error":                "无法连接到配置的上游。",
	"upstream_timeout":              "上游未能在规定时间内返回响应头。",
	"menu_status_starting":          "正在启动",
	"menu_status_running":           "运行于 %s",
	"menu_status_config_error":      "配置错误",
	"menu_status_config_error_at":   "配置错误；仍运行于 %s",
	"menu_status_listener_error":    "监听错误",
	"menu_status_listener_error_at": "监听错误：%s",
	"menu_status_stopped":           "已停止",
	"menu_models":                   "模型数：%d",
	"menu_logging_unavailable":      "日志不可用",
	"menu_open_config":              "打开配置",
	"menu_reload":                   "重新加载配置",
	"menu_open_logs":                "打开日志",
	"menu_autostart":                "开机自动启动",
	"menu_quit":                     "退出",
	"tooltip":                       "Kitsune Proxy",
}
