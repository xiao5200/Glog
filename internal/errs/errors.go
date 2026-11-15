package errs

import (
	"errors"
	"fmt"
)

// 领域错误（可用 errors.Is 判断）。
var (
	ErrNotFound         = errors.New("资源不存在")
	ErrAlreadyExists    = errors.New("资源已存在")
	ErrInvalidInput     = errors.New("无效输入")
	ErrPermissionDenied = errors.New("权限被拒绝")

	ErrProjectNotOpen   = errors.New("未打开项目")
	ErrProjectCorrupted = errors.New("项目文件损坏")
	ErrProjectExists    = errors.New("项目已存在")

	ErrSSHConnectionFailed = errors.New("SSH连接失败")
	ErrFileNotFound        = errors.New("文件不存在")
	ErrUnsupportedOS       = errors.New("当前操作系统不支持该操作")

	ErrParseTemplateMissing = errors.New("解析模板缺失")
	ErrParseFormatUnknown   = errors.New("未知日志格式")
	ErrParseFailed          = errors.New("日志解析失败")

	ErrIndexCorrupted = errors.New("索引文件损坏")
	ErrIndexNotBuilt  = errors.New("索引未构建")

	ErrQuerySyntaxError = errors.New("查询语法错误")
	ErrQueryTimeout     = errors.New("查询超时")

	ErrAIProviderNotConfigured = errors.New("AI提供商未配置")
	ErrAIRequestFailed         = errors.New("AI请求失败")
	ErrAIQuotaExceeded         = errors.New("AI配额超限")
	ErrAIInvalidResponse       = errors.New("AI返回内容无法解析")

	ErrRuleInvalid    = errors.New("检测规则格式错误")
	ErrIOCInvalid     = errors.New("IOC格式错误")
	ErrArchiveInvalid = errors.New("归档包无效")
)

// 错误码。
const (
	CodeInternal     = "INTERNAL_ERROR"
	CodeNotFound     = "NOT_FOUND"
	CodeInvalidInput = "INVALID_INPUT"
	CodeSSHFailed    = "SSH_FAILED"
	CodeParseFailed  = "PARSE_FAILED"
	CodeQueryFailed  = "QUERY_FAILED"
	CodeAIFailed     = "AI_FAILED"
	CodeRuleFailed   = "RULE_FAILED"
	CodeIOFailed     = "IO_FAILED"
)

// AppError 带上下文的领域错误。
type AppError struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Err     error                  `json:"-"`
	Context map[string]interface{} `json:"context,omitempty"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error { return e.Err }

func New(code, message string, err error) *AppError {
	return &AppError{Code: code, Message: message, Err: err}
}

func NewWithContext(code, message string, err error, ctx map[string]interface{}) *AppError {
	return &AppError{Code: code, Message: message, Err: err, Context: ctx}
}

// InvalidInput 构造输入错误。
func InvalidInput(message string, err error) *AppError {
	return New(CodeInvalidInput, message, err)
}

// NotFound 构造未找到错误。
func NotFound(message string, err error) *AppError {
	return New(CodeNotFound, message, err)
}

// Internal 构造内部错误。
func Internal(message string, err error) *AppError {
	return New(CodeInternal, message, err)
}

// As 提取 AppError。
func As(err error) (*AppError, bool) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr, true
	}
	return nil, false
}
