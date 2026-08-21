package douyinLive

import (
	"errors"
	"strings"
)

// ErrInvalidLiveID indicates that a live-room identifier is empty, too long, or contains unsupported characters.
// ErrInvalidLiveID 表示直播间标识为空、过长或包含不支持的字符。
var ErrInvalidLiveID = errors.New("直播间标识无效")

// ValidateLiveID validates and normalizes a Douyin live-room identifier.
// ValidateLiveID 校验并规范化抖音直播间标识。
//
// Valid identifiers contain 1-128 ASCII letters, digits, underscores, or
// hyphens. The returned value has surrounding whitespace removed.
// 有效标识由 1-128 个 ASCII 字母、数字、下划线或短横线组成；返回值会移除首尾空白。
func ValidateLiveID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 128 {
		return "", ErrInvalidLiveID
	}
	for _, ch := range value {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '_' || ch == '-' {
			continue
		}
		return "", ErrInvalidLiveID
	}
	return value, nil
}
