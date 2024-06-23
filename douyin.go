package douyinlive

import (
	"DouyinLive/generated/douyin"
	"DouyinLive/global"
	"DouyinLive/utils"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"github.com/elliotchance/orderedmap"
	"github.com/gorilla/websocket"
	"github.com/imroc/req/v3"
	"github.com/spf13/cast"
	"google.golang.org/protobuf/proto"
	"io"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func NewDouyinLive(liveid string) (*DouyinLive, error) {

	ua := GetRandomUA()

	c := req.C().SetUserAgent(ua)
	d := &DouyinLive{
		ttwid:         "",
		roomid:        "",
		liveid:        liveid,
		liveurl:       "https://live.douyin.com/",
		Useragent:     ua,
		c:             c,
		eventHandlers: make([]EventHandler, 0),
	}
	var err error
	d.ttwid, err = d.fttwid()

	if err != nil {
		return nil, err
	}
	d.roomid = d.froomid()
	return d, nil
}
func GetxMSStub(params *orderedmap.OrderedMap) string {
	// 使用 strings.Builder 构建签名字符串
	var sigParams strings.Builder
	first := true
	for _, key := range params.Keys() {
		if !first {
			sigParams.WriteString(",")
		} else {
			first = false
		}
		value, _ := params.Get(key)
		sigParams.WriteString(fmt.Sprintf("%s=%s", key, value))
	}
	hash := md5.Sum([]byte(sigParams.String()))
	return hex.EncodeToString(hash[:])
}
func GetRandomUA() string {
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
	chromeVersion := chromeVersionList[rand.Intn(len(chromeVersionList))]
	//return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36"
	return fmt.Sprintf("Mozilla/5.0 %s AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", os, chromeVersion)
}
func randomUserAgent(device string) string {
	chromeVersion := rand.Intn(20) + 100 // 生成 100 到 120 之间的随机 Chrome 版本号

	switch device {
	case "mobile":
		androidVersion := rand.Intn(6) + 9 // 生成 9 到 14 之间的随机 Android 版本号
		mobileModels := []string{
			"SM-G981B", "SM-G9910", "SM-S9080", "SM-S9110", "SM-S921B",
			"Pixel 5", "Pixel 6", "Pixel 7", "Pixel 7 Pro", "Pixel 8",
		}
		mobileModel := mobileModels[rand.Intn(len(mobileModels))]
		return fmt.Sprintf("Mozilla/5.0 (Linux; Android %d; %s) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.0.0 Mobile Safari/537.36", androidVersion, mobileModel, chromeVersion)
	default:
		return fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%d.0.0.0 Safari/537.36", chromeVersion)
	}
}
func getUserID() string {
	// 生成7300000000000000000到7999999999999999999之间的随机数
	randomNumber := rand.Int63n(7000000000000000000 + 1)

	// 将整数转换为字符串
	return strconv.FormatInt(randomNumber, 10)
}
func (d *DouyinLive) Start() {
	var err error

	smap := orderedmap.NewOrderedMap()
	smap.Set("live_id", "1")
	smap.Set("aid", "6383")
	smap.Set("version_code", "180800")
	smap.Set("webcast_sdk_version", "1.0.14-beta.0")
	smap.Set("room_id", d.roomid)
	smap.Set("sub_room_id", "")
	smap.Set("sub_channel_id", "")
	smap.Set("did_rule", "3")
	smap.Set("user_unique_id", d.pushid)
	smap.Set("device_platform", "web")
	smap.Set("device_type", "")
	smap.Set("ac", "")
	smap.Set("identity", "audience")
	signaturemd5 := GetxMSStub(smap)

	signature := global.GetSing(signaturemd5)

	browserInfo := strings.Split(d.Useragent, "Mozilla")[1]
	parsedURL := strings.Replace(browserInfo[1:], " ", "%20", -1)
	fetchTime := time.Now().UnixNano() / int64(time.Millisecond)

	browserVersion := parsedURL

	wssURL := "wss://webcast5-ws-web-lf.douyin.com/webcast/im/push/v2/?app_name=douyin_web&version_code=180800&" +
		"webcast_sdk_version=1.0.14-beta.0&update_version_code=1.0.14-beta.0&compress=gzip&device_platform" +
		"=web&cookie_enabled=true&screen_width=1920&screen_height=1080&browser_language=zh-CN&browser_platform=Win32&" +
		"browser_name=Mozilla&browser_version=" + browserVersion + "&browser_online=true" +
		"&tz_name=Asia/Shanghai&cursor=d-1_u-1_fh-7383731312643626035_t-1719159695790_r-1&internal_ext" +
		"=internal_src:dim|wss_push_room_id:" + d.roomid + "|wss_push_did:" + d.pushid + "|first_req_ms:" + cast.ToString(fetchTime) + "|fetch_time:" + cast.ToString(fetchTime) + "|seq:1|wss_info:0-" + cast.ToString(fetchTime) + "-0-0|" +
		"wrds_v:7382620942951772256&host=https://live.douyin.com&aid=6383&live_id=1&did_rule=3" +
		"&endpoint=live_pc&support_wrds=1&user_unique_id=" + d.pushid + "&im_path=/webcast/im/fetch/" +
		"&identity=audience&need_persist_msg_count=15&insert_task_id=&live_reason=&room_id=" + d.roomid + "&heartbeatDuration=0&signature=" + signature

	headers := make(http.Header)
	headers.Add("user-agent", d.Useragent)
	headers.Add("cookie", fmt.Sprintf("ttwid=%s", d.ttwid))
	var response *http.Response
	d.Conn, response, err = websocket.DefaultDialer.Dial(wssURL, headers)

	if err != nil {
		log.Printf("链接失败: err:%v\nroomid:%v\n ttwid:%v\nwssurl:----%v\nresponse:%v\n", err, d.roomid, d.ttwid, wssURL, response.StatusCode)
		return

	}
	log.Println("链接成功")

	for {
		_, message, err := d.Conn.ReadMessage()
		if err != nil {
			log.Println("读取消息失败-可能已经关播：", err, message)
			return
		}

		if message != nil {
			pac := &douyin.PushFrame{}
			err := proto.Unmarshal(message, pac)
			if err != nil {
				log.Println("解析消息失败：", err)
				continue
			}
			n := false
			for _, v := range pac.HeadersList {
				if v.Key == "compress_type" {
					if v.Value == "gzip" {
						n = true
						continue
					}
				}
			}
			//消息为gzip压缩进行处理.否则抛弃
			if n == true && pac.PayloadType == "msg" {
				gzipReader, err := gzip.NewReader(bytes.NewReader(pac.Payload))
				if err != nil {
					log.Println("Gzip加载失败:", err)
					continue
				}
				uncompressedData, err := io.ReadAll(gzipReader)
				if err != nil {
					log.Println("数据流加载失败:", err)
					continue
				}
				response := &douyin.Response{}
				err = proto.Unmarshal(uncompressedData, response)
				if err != nil {
					log.Println("解析消息失败：", err)
					continue
				}
				defer gzipReader.Close()
				if response.NeedAck {
					ack := &douyin.PushFrame{
						LogId:       pac.LogId,
						PayloadType: "ack",
						Payload:     []byte(response.InternalExt),
					}
					serializedAck, err := proto.Marshal(ack)
					if err != nil {
						log.Println("proto心跳包序列化失败:", err)
					}
					err = d.Conn.WriteMessage(websocket.BinaryMessage, serializedAck)
					if err != nil {
						fmt.Println("心跳包发送失败：", err)
					}
					//fmt.Println("心跳包发送成功", ack)
				}
				//log.Println(d.eventHandlers)

				d.ProcessingMessage(response)
			}

		}
	}
}
func (d *DouyinLive) Emit(eventData *douyin.Message) {
	for _, handler := range d.eventHandlers {
		handler(eventData)
	}
}
func (d *DouyinLive) ProcessingMessage(response *douyin.Response) {

	for _, data := range response.MessagesList {
		//method := data.Method
		d.Emit(data)
		//
		//switch method {
		//
		//case "WebcastChatMessage":
		//	//msg := &douyin.ChatMessage{}
		//	//proto.Unmarshal(data.Payload, msg)
		//	//log.Println("聊天msg", msg.User.Id, msg.User.NickName, msg.Content)
		//case "WebcastGiftMessage":
		//	//msg := &douyin.GiftMessage{}
		//	//proto.Unmarshal(data.Payload, msg)
		//	//log.Println("礼物msg", msg.User.Id, msg.User.NickName, msg.Gift.Name, msg.ComboCount)
		//case "WebcastLikeMessage":
		//	//msg := &douyin.LikeMessage{}
		//	//proto.Unmarshal(data.Payload, msg)
		//	//log.Println("点赞msg", msg.User.Id, msg.User.NickName, msg.Count)
		//case "WebcastMemberMessage":
		//	//msg := &douyin.MemberMessage{}
		//	//proto.Unmarshal(data.Payload, msg)
		//	//log.Println("进场msg", msg.User.Id, msg.User.NickName, msg.User.Gender)
		//case "WebcastSocialMessage":
		//	//msg := &douyin.SocialMessage{}
		//	//proto.Unmarshal(data.Payload, msg)
		//	//log.Println("关注msg", msg.User.Id, msg.User.NickName)
		//case "WebcastRoomUserSeqMessage":
		//	//msg := &douyin.RoomUserSeqMessage{}
		//	//proto.Unmarshal(data.Payload, msg)
		//	//log.Printf("房间人数msg 当前观看人数:%v,累计观看人数:%v\n", msg.Total, msg.TotalPvForAnchor)
		//case "WebcastFansclubMessage":
		//	//msg := &douyin.FansclubMessage{}
		//	//proto.Unmarshal(data.Payload, msg)
		//	//log.Printf("粉丝团msg %v\n", msg.Content)
		//case "WebcastControlMessage":
		//	//msg := &douyin.ControlMessage{}
		//	//proto.Unmarshal(data.Payload, msg)
		//	//log.Printf("直播间状态消息%v", msg.Status)
		//case "WebcastEmojiChatMessage":
		//	//msg := &douyin.EmojiChatMessage{}
		//	//proto.Unmarshal(data.Payload, msg)
		//	//log.Printf("表情消息%vuser:%vcommon:%vdefault_content:%v", msg.EmojiId, msg.User, msg.Common, msg.DefaultContent)
		//case "WebcastRoomStatsMessage":
		//	//msg := &douyin.RoomStatsMessage{}
		//	//proto.Unmarshal(data.Payload, msg)
		//	//log.Printf("直播间统计msg%v", msg.DisplayLong)
		//case "WebcastRoomMessage":
		//	//msg := &douyin.RoomMessage{}
		//	//proto.Unmarshal(data.Payload, msg)
		//	//log.Printf("【直播间msg】直播间id%v", msg.Common.RoomId)
		//case "WebcastRoomRankMessage":
		//	//msg := &douyin.RoomRankMessage{}
		//	//proto.Unmarshal(data.Payload, msg)
		//	//log.Printf("直播间排行榜msg%v", msg.RanksList)
		//
		//default:
		//	//d.Emit(Default, data.Payload)
		//	//log.Println("payload:", method, hex.EncodeToString(data.Payload))
		//}

	}
	//fmt.Println("消息", message)
}
func (d *DouyinLive) Subscribe(handler EventHandler) {
	d.eventHandlers = append(d.eventHandlers, handler)
}

func (d *DouyinLive) fttwid() (string, error) {
	if d.ttwid != "" {
		return d.ttwid, nil
	}
	res, err := d.c.R().Get(d.liveurl)
	if err != nil {
		return "", err
	}
	var ttwidCookie http.Cookie
	for _, cookie := range res.Cookies() {
		if cookie.Name == "ttwid" {
			//fmt.Println(cookie.Name, cookie.Value)
			ttwidCookie = *cookie
			d.ttwid = cookie.Value
			break
		}
	}
	return ttwidCookie.Value, nil
}

func (d *DouyinLive) froomid() string {
	if d.roomid != "" {
		return d.roomid

	}
	t, _ := d.fttwid()
	ttwid := &http.Cookie{
		Name:  "ttwid",
		Value: "ttwid=" + t + "&msToken=" + utils.GenerateMsToken(107),
	}
	acNonce := &http.Cookie{
		Name:  "__ac_nonce",
		Value: "0123407cc00a9e438deb4",
	}
	//cookie := "ttwid=" + t.Value + "&msToken=" + utils.GenerateMsToken(107) + "; __ac_nonce=0123407cc00a9e438deb4"
	res, err := d.c.R().SetCookies(ttwid, acNonce).Get(d.liveurl + d.liveid)

	if err != nil {
		return err.Error()

	}
	re := regexp.MustCompile(`roomId\\":\\"(\d+)\\"`)
	match := re.FindStringSubmatch(res.String())

	d.roomid = d.regroomid(res.String())
	d.pushid = d.regpushid(res.String())
	return match[1]

}
func (d *DouyinLive) regroomid(s string) string {
	re := regexp.MustCompile(`roomId\\":\\"(\d+)\\"`)
	match := re.FindStringSubmatch(s)
	return match[1]
}
func (d *DouyinLive) regpushid(s string) string {
	re := regexp.MustCompile(`user_unique_id\\":\\"(\d+)\\"`)
	match := re.FindStringSubmatch(s)
	return match[1]
}
