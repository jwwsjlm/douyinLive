package utils

import (
	"DouyinLive/generated/douyin"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/elliotchance/orderedmap"
	"google.golang.org/protobuf/reflect/protoreflect"
	"io"
	"log"
	"math/rand"
	"strconv"
	"strings"
)

// HasGzipEncoding 判断消息体是否包含gzip
func HasGzipEncoding(h []*douyin.HeadersList) bool {
	n := false
	for _, v := range h {
		if v.Key == "compress_type" {
			if v.Value == "gzip" {
				n = true
				continue
			}
		}
	}
	return n
}

// GzipUnzip 解压gzip
func GzipUnzip(compressedData []byte) ([]byte, error) {
	// 创建一个读取gzip数据的reader
	reader, err := gzip.NewReader(bytes.NewReader(compressedData))
	if err != nil {
		return nil, err
	}
	defer func(reader *gzip.Reader) {
		err := reader.Close()
		if err != nil {
			log.Printf("Failed to close gzip reader: %v", err)
		}
	}(reader)
	// 创建一个buffer来存储解压后的数据
	var buffer bytes.Buffer
	// 将解压的数据写入buffer
	_, err = io.Copy(&buffer, reader)
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// GetxMSStub 拼接map返回md5.hex
func GetxMSStub(params *orderedmap.OrderedMap) string {
	// 使用 strings.Builder 构建签名字符串
	var sigParams strings.Builder
	first := true
	for _, key := range params.Keys() {
		if !first {
			// 如果不是第一个参数，需要在参数之间加上逗号
			sigParams.WriteString(",")

		} else {

			first = false

		}
		//
		value, _ := params.Get(key)
		sigParams.WriteString(fmt.Sprintf("%s=%s", key, value))
	}
	//
	hash := md5.Sum([]byte(sigParams.String()))

	//
	return hex.EncodeToString(hash[:])
}

func getUserID() string {
	// 生成7300000000000000000到7999999999999999999之间的随机数
	randomNumber := rand.Int63n(7000000000000000000 + 1)
	// 将整数转换为字符串
	return strconv.FormatInt(randomNumber, 10)
}

// GenerateMsToken 获得随机生成的token
func GenerateMsToken(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+="

	b := make([]byte, length)
	for i := 0; i < length; i++ {
		// 生成0到charset长度之间的随机数
		randInt := rand.Intn(len(charset))

		// 将随机数转换为字符集中的字符
		b[i] = charset[randInt]
	}

	return string(b) + "=_"
}

// MatchMethod 匹配处理函数
func MatchMethod(Method string) (protoreflect.ProtoMessage, error) {
	switch Method {
	case "WebcastChatMessage":

		return &douyin.ChatMessage{}, nil

	case "WebcastGiftMessage":
		return &douyin.GiftMessage{}, nil

	case "WebcastLikeMessage":
		return &douyin.LikeMessage{}, nil
	case "WebcastMemberMessage":
		return &douyin.MemberMessage{}, nil
	case "WebcastSocialMessage":
		return &douyin.SocialMessage{}, nil
	case "WebcastRoomUserSeqMessage":
		return &douyin.RoomUserSeqMessage{}, nil
	case "WebcastFansclubMessage":
		return &douyin.FansclubMessage{}, nil
	case "WebcastControlMessage":
		return &douyin.ControlMessage{}, nil
	case "WebcastEmojiChatMessage":
		return &douyin.EmojiChatMessage{}, nil
	case "WebcastRoomStatsMessage":
		return &douyin.RoomStatsMessage{}, nil
	case "WebcastRoomMessage":
		return &douyin.RoomMessage{}, nil
	case "WebcastRanklistHourEntranceMessage":
		return &douyin.RanklistHourEntranceMessage{}, nil
	case "WebcastRoomRankMessage":
		return &douyin.RoomRankMessage{}, nil
	case "WebcastInRoomBannerMessage":
		return &douyin.InRoomBannerMessage{}, nil
	case "WebcastRoomDataSyncMessage":
		return &douyin.RoomDataSyncMessage{}, nil
	case "WebcastLuckyBoxTempStatusMessage":
		return &douyin.LuckyBoxTempStatusMessage{}, nil
	case "WebcastDecorationModifyMethod":
		return &douyin.DecorationModifyMessage{}, nil
	case "WebcastLinkMicAudienceKtvMessage":
		return &douyin.LinkMicAudienceKtvMessage{}, nil
	case "WebcastRoomStreamAdaptationMessage":
		return &douyin.RoomStreamAdaptationMessage{}, nil
	case "WebcastQuizAudienceStatusMessage":
		return &douyin.QuizAudienceStatusMessage{}, nil
	case "WebcastHotChatMessage":
		return &douyin.HotChatMessage{}, nil
	case "WebcastHotRoomMessage":
		return &douyin.HotRoomMessage{}, nil
	case "WebcastAudioChatMessage":
		return &douyin.AudioChatMessage{}, nil
	case "WebcastRoomNotifyMessage":
		return &douyin.NotifyMessage{}, nil
	case "WebcastLuckyBoxMessage":
		return &douyin.LuckyBoxMessage{}, nil
	case "WebcastUpdateFanTicketMessage":
		return &douyin.UpdateFanTicketMessage{}, nil
	case "WebcastScreenChatMessage":
		return &douyin.ScreenChatMessage{}, nil
	case "WebcastNotifyEffectMessage":
		return &douyin.NotifyEffectMessage{}, nil
	case "WebcastBindingGiftMessage":
		return &douyin.NotifyEffectMessage_BindingGiftMessage{}, nil
	case "WebcastTempStateAreaReachMessage":
		return &douyin.TempStateAreaReachMessage{}, nil
	case "WebcastGrowthTaskMessage":
		return &douyin.GrowthTaskMessage{}, nil
	case "WebcastGameCPBaseMessage":
		return &douyin.GameCPBaseMessage{}, nil
	default:
		return nil, errors.New("未知消息:" + Method)
	}
}

// GzipCompressAndBase64Encode 消息进行gzip压缩转为base64
func GzipCompressAndBase64Encode(data []byte) (string, error) {
	// 创建 gzip 压缩器
	var b bytes.Buffer
	w := gzip.NewWriter(&b)

	// 压缩数据
	_, err := w.Write(data)
	if err != nil {
		return "", err
	}

	// 关闭压缩器
	if err := w.Close(); err != nil {
		return "", err
	}

	// 获取压缩后的数据
	compressedData := b.Bytes()

	// 进行 Base64 编码
	encodedData := base64.StdEncoding.EncodeToString(compressedData)

	return encodedData, nil
}
func NewOrderedMap(r, p string) *orderedmap.OrderedMap {
	smap := orderedmap.NewOrderedMap()
	smap.Set("live_id", "1")
	smap.Set("aid", "6383")
	smap.Set("version_code", "180800")
	smap.Set("webcast_sdk_version", "1.0.14-beta.0")
	smap.Set("room_id", r)
	smap.Set("sub_room_id", "")
	smap.Set("sub_channel_id", "")
	smap.Set("did_rule", "3")
	smap.Set("user_unique_id", p)
	smap.Set("device_platform", "web")
	smap.Set("device_type", "")
	smap.Set("ac", "")
	smap.Set("identity", "audience")
	return smap
}

// RandomUserAgent 随机浏览器UA
func RandomUserAgent() string {
	osList := []string{
		"(Windows NT 10.0; WOW64)", "(Windows NT 10.0; WOW64)", "(Windows NT 10.0; Win64; x64)",
		"(Windows NT 6.3; WOW64)", "(Windows NT 6.3; Win64; x64)",
		"(Windows NT 6.1; Win64; x64)", "(Windows NT 6.1; WOW64)",
		"(X11; Linux x86_64)",
		"(Macintosh; Intel Mac OS X 10_12_6)",
	}

	chromeVersionList := []string{
		"110.0.5481.77", "110.0.5481.30", "109.0.5414.74", "108.0.5359.71", "108.0.5359.22",
		// ... 其他版本号
		"98.0.4758.48", "97.0.4692.71",
	}

	os := osList[rand.Intn(len(osList))]
	//

	chromeVersion := chromeVersionList[rand.Intn(len(chromeVersionList))]
	//return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
	return fmt.Sprintf("Mozilla/5.0 %s AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", os, chromeVersion)
}
