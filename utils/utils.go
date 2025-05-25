package utils

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/elliotchance/orderedmap"
	"github.com/google/uuid"
	"github.com/jwwsjlm/douyinLive/generated/new_douyin"
	"math/rand"
	"strconv"
	"strings"
)

// HasGzipEncoding 判断消息头中是否包含gzip编码

func HasGzipEncoding(headers []*new_douyin.Webcast_Im_PushHeader) bool {

	for _, header := range headers {
		if header.Key == "compress_type" && header.Value == "gzip" {
			return true
		}
	}
	return false
}

// GetxMSStub 拼接map并返回其MD5哈希值的十六进制字符串
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

// getUserID 生成随机用户ID
func getUserID() string {
	// 生成7300000000000000000到7999999999999999999之间的随机数
	randomNumber := rand.Int63n(7000000000000000000 + 1)
	// 将整数转换为字符串
	return strconv.FormatInt(randomNumber, 10)
}

// GenerateMsToken 生成随机的msToken
func GenerateMsToken(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+="
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b) + "=_"
}

// GzipCompressAndBase64Encode 将数据进行gzip压缩并进行Base64编码
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

// NewOrderedMap 创建一个有序的map
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

// RandomUserAgent 生成随机的浏览器用户代理字符串
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

	os := osList[rand.Intn(len(osList))]
	chromeVersion := chromeVersionList[rand.Intn(len(chromeVersionList))]

	return fmt.Sprintf("Mozilla/5.0 %s AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", os, chromeVersion)
}

// GenerateUniqueID 生成唯一标识符
func GenerateUniqueID() string {
	return uuid.New().String()
}
