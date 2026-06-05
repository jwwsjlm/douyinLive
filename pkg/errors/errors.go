package errors

import (
	"fmt"
)

// ErrorCode 定义错误码类型
type ErrorCode int

const (
	// 系统错误
	ErrCodeSystem ErrorCode = iota + 1000
	ErrCodeNetwork
	ErrCodeTimeout

	// 业务错误
	ErrCodeRoomNotFound ErrorCode = iota + 2000
	ErrCodeRoomNotLive
	ErrCodeInvalidParam

	// WebSocket 错误
	ErrCodeWSConnection ErrorCode = iota + 3000
	ErrCodeWSUpgrade
	ErrCodeWSMessage
)

// AppError 应用错误结构
type AppError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Cause   error     `json:"-"`
}

func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// New 创建新的应用错误
func New(code ErrorCode, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
	}
}

// Wrap 包装现有错误
func Wrap(err error, code ErrorCode, message string) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Cause:   err,
	}
}

// 预定义错误
var (
	ErrRoomNotFound = New(ErrCodeRoomNotFound, "直播间未找到")
	ErrRoomNotLive  = New(ErrCodeRoomNotLive, "直播间未开播")
	ErrInvalidParam = New(ErrCodeInvalidParam, "参数无效")
)
