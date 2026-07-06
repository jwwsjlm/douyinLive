package utils

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	cryptorand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/elliotchance/orderedmap"
	"github.com/google/uuid"
	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
)

// HasGzipEncoding 判断消息头中是否包含 gzip 编码。
// HasGzipEncoding reports whether the push headers declare gzip compression.
func HasGzipEncoding(headers []*new_douyin.Webcast_Im_PushHeader) bool {
	for _, header := range headers {
		if header.Key == "compress_type" && header.Value == "gzip" {
			return true
		}
	}
	return false
}

// GetxMSStub 拼接有序参数并返回 MD5 十六进制摘要。
// GetxMSStub joins ordered parameters and returns their MD5 hex digest.
func GetxMSStub(params *orderedmap.OrderedMap) string {
	var sigParams strings.Builder
	for i, key := range params.Keys() {
		if i > 0 {
			sigParams.WriteString(",")
		}
		value, _ := params.Get(key)
		sigParams.WriteString(fmt.Sprintf("%s=%s", key, value))
	}
	hash := md5.Sum([]byte(sigParams.String()))
	return hex.EncodeToString(hash[:])
}

// randomIndex 返回 [0, max) 范围内的安全随机索引。
// randomIndex returns a cryptographically random index in the [0, max) range.
func randomIndex(max int) int {
	if max <= 1 {
		return 0
	}

	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0
	}
	return int(n.Int64())
}

// randomInt64 返回 [0, max) 范围内的安全随机 int64。
// randomInt64 returns a cryptographically random int64 in the [0, max) range.
func randomInt64(max int64) int64 {
	if max <= 1 {
		return 0
	}

	n, err := cryptorand.Int(cryptorand.Reader, big.NewInt(max))
	if err != nil {
		return 0
	}
	return n.Int64()
}

// GenerateJitterNanos 生成不超过最大时长的随机抖动纳秒数。
// GenerateJitterNanos returns random jitter in nanoseconds up to the maximum duration.
func GenerateJitterNanos(maxDuration time.Duration) int64 {
	if maxDuration <= 0 {
		return 0
	}
	return randomInt64(int64(maxDuration))
}

// GenerateMsToken 生成随机 msToken。
// GenerateMsToken generates a random msToken value.
func GenerateMsToken(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+="
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[randomIndex(len(charset))]
	}
	return string(b) + "=_"
}

// GzipCompressAndBase64Encode 将数据 gzip 压缩后进行 Base64 编码。
// GzipCompressAndBase64Encode gzip-compresses data and encodes it as Base64.
func GzipCompressAndBase64Encode(data []byte) (string, error) {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)

	if _, err := w.Write(data); err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

// NewOrderedMap 创建 WebSocket 签名所需的有序参数表。
// NewOrderedMap creates the ordered parameter map required for WebSocket signing.
func NewOrderedMap(roomID, pushID string) *orderedmap.OrderedMap {
	smap := orderedmap.NewOrderedMap()
	smap.Set("live_id", "1")
	smap.Set("aid", "6383")
	smap.Set("version_code", "180800")
	smap.Set("webcast_sdk_version", "1.0.14-beta.0")
	smap.Set("room_id", roomID)
	smap.Set("sub_room_id", "")
	smap.Set("sub_channel_id", "")
	smap.Set("did_rule", "3")
	smap.Set("user_unique_id", pushID)
	smap.Set("device_platform", "web")
	smap.Set("device_type", "")
	smap.Set("ac", "")
	smap.Set("identity", "audience")
	return smap
}

// RandomUserAgent 生成随机浏览器 User-Agent。
// RandomUserAgent generates a random browser User-Agent string.
func RandomUserAgent() string {
	osList := []string{
		"(Windows NT 10.0; WOW64)", "(Windows NT 10.0; Win64; x64)",
		"(Windows NT 6.3; WOW64)", "(Windows NT 6.3; Win64; x64)",
		"(Windows NT 6.1; Win64; x64)", "(Windows NT 6.1; WOW64)",
		"(X11; Linux x86_64)",
		"(Macintosh; Intel Mac OS X 10_12_6)",
	}

	chromeVersionList := []string{
		"110.0.5481.77", "110.0.5481.30", "109.0.5414.74", "108.0.5359.71",
		"108.0.5359.22", "98.0.4758.48", "97.0.4692.71",
	}

	os := osList[randomIndex(len(osList))]
	chromeVersion := chromeVersionList[randomIndex(len(chromeVersionList))]

	return fmt.Sprintf("Mozilla/5.0 %s AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", os, chromeVersion)
}

// GenerateUniqueID 生成唯一标识符。
// GenerateUniqueID generates a unique identifier.
func GenerateUniqueID() string {
	return uuid.New().String()
}
