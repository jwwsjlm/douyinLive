package douyinlive

import (
	"DouyinLive/generated/douyin"
	"DouyinLive/global"
	"DouyinLive/utils"
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/imroc/req/v3"
	"github.com/spf13/cast"
	"google.golang.org/protobuf/proto"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// NewDouyinLive 创建一个新的链接
func NewDouyinLive(liveid string) (*DouyinLive, error) {
	var err error
	ua := utils.RandomUserAgent()

	c := req.C().SetUserAgent(ua)
	d := &DouyinLive{
		ttwid:         "",
		roomid:        "",
		liveid:        liveid,
		liveurl:       "https://live.douyin.com/",
		Useragent:     ua,
		c:             c,
		eventHandlers: make([]EventHandler, 0),
		headers:       http.Header{},
	}
	d.ttwid, err = d.fttwid()

	if err != nil {
		return nil, err
	}
	d.roomid = d.froomid()
	return d, nil
}

func (d *DouyinLive) Link() {

}

func (d *DouyinLive) reconnect(i int) bool {
	var err error
	log.Println("尝试重新连接...")
	for attempt := 0; attempt < i; attempt++ {
		if d.Conn != nil {
			err := d.Conn.Close() // 关闭当前连接
			if err != nil {
				log.Printf("关闭连接失败: %v", err)
			}
		}
		// 重新建立连接
		d.Conn, _, err = websocket.DefaultDialer.Dial(d.wssurl, d.headers)
		if err != nil {
			log.Printf("重连失败: %v", err)
			log.Printf("正在尝试第 %d 次重连...", attempt+1)
			time.Sleep(5 * time.Second) // 等待5秒后重试
		} else {
			log.Println("重连成功")
			return true // 重连成功，返回true
		}
	}
	log.Println("重连失败，退出程序")
	return false // 重连失败，返回false
}
func (d *DouyinLive) StitchUrl() string {
	smap := utils.NewOrderedMap(d.roomid, d.pushid)
	signaturemd5 := utils.GetxMSStub(smap)
	signature := global.GetSing(signaturemd5)
	browserInfo := strings.Split(d.Useragent, "Mozilla")[1]
	parsedURL := strings.Replace(browserInfo[1:], " ", "%20", -1)
	fetchTime := time.Now().UnixNano() / int64(time.Millisecond)
	return "wss://webcast5-ws-web-lf.douyin.com/webcast/im/push/v2/?app_name=douyin_web&version_code=180800&" +
		"webcast_sdk_version=1.0.14-beta.0&update_version_code=1.0.14-beta.0&compress=gzip&device_platform" +
		"=web&cookie_enabled=true&screen_width=1920&screen_height=1080&browser_language=zh-CN&browser_platform=Win32&" +
		"browser_name=Mozilla&browser_version=" + parsedURL + "&browser_online=true" +
		"&tz_name=Asia/Shanghai&cursor=d-1_u-1_fh-7383731312643626035_t-1719159695790_r-1&internal_ext" +
		"=internal_src:dim|wss_push_room_id:" + d.roomid + "|wss_push_did:" + d.pushid + "|first_req_ms:" + cast.ToString(fetchTime) + "|fetch_time:" + cast.ToString(fetchTime) + "|seq:1|wss_info:0-" + cast.ToString(fetchTime) + "-0-0|" +
		"wrds_v:7382620942951772256&host=https://live.douyin.com&aid=6383&live_id=1&did_rule=3" +
		"&endpoint=live_pc&support_wrds=1&user_unique_id=" + d.pushid + "&im_path=/webcast/im/fetch/" +
		"&identity=audience&need_persist_msg_count=15&insert_task_id=&live_reason=&room_id=" + d.roomid + "&heartbeatDuration=0&signature=" + signature
}
func (d *DouyinLive) GzipUnzip(compressedData []byte) ([]byte, error) {
	//log.Println(compressedData)

	var err error
	d.gzip, err = gzip.NewReader(bytes.NewReader(compressedData))
	if err != nil {
		return nil, err
	}

	decompressedData, err := io.ReadAll(d.gzip)

	if err != nil {
		// 读取失败，关闭 gzip 读取器
		d.gzip.Close()
		d.gzip = nil
		return nil, err
	}
	//log.Println(string(decompressedData))
	return decompressedData, nil

}

// GzipUnzipReset 使用Reset解压gzip
func (d *DouyinLive) GzipUnzipReset(compressedData []byte) ([]byte, error) {
	//log.Println(compressedData)
	var err error
	if d.gzip != nil {
		err := d.gzip.Reset(bytes.NewReader(compressedData))
		if err != nil {
			d.gzip.Close()
			d.gzip = nil
			return nil, err
		}
	} else {
		d.gzip, err = gzip.NewReader(bytes.NewReader(compressedData))
		if err != nil {
			return nil, err
		}
	}

	decompressedData, err := io.ReadAll(d.gzip)

	if err != nil {
		// 读取失败，关闭 gzip 读取器
		d.gzip.Close()
		d.gzip = nil
		return nil, err
	}
	//log.Println(string(decompressedData))
	return decompressedData, nil

}

// Start 开始运行
func (d *DouyinLive) Start() {
	var err error
	//链接地址
	//gzipReader, err := gzip.NewReader(nil)

	d.wssurl = d.StitchUrl()
	d.headers.Add("user-agent", d.Useragent)
	d.headers.Add("cookie", fmt.Sprintf("ttwid=%s", d.ttwid))
	var response *http.Response
	d.Conn, response, err = websocket.DefaultDialer.Dial(d.wssurl, d.headers)
	if err != nil {
		log.Printf("链接失败: err:%v\nroomid:%v\n ttwid:%v\nwssurl:----%v\nresponse:%v\n", err, d.roomid, d.ttwid, d.wssurl, response.StatusCode)
	}
	log.Println("链接成功")
	defer d.gzip.Close()
	for {
		//读取消息
		messageType, message, err := d.Conn.ReadMessage()
		if err != nil {
			log.Println("读取消息失败-", err, message, messageType)
			//进行重连
			if d.reconnect(5) {
				continue // 如果重连成功，继续监听消息
			} else {
				break
			}
			//log.Println("读取消息失败-", err, message, messageType)
			//break

		} else {
			//解析消息正常的话进行处理
			if message != nil {
				pac := &douyin.PushFrame{}
				err := proto.Unmarshal(message, pac)
				if err != nil {

					log.Println("解析消息失败：", err)
					continue
				}

				n := utils.HasGzipEncoding(pac.HeadersList)
				//消息为gzip压缩进行处理.否则抛弃
				if n == true && pac.PayloadType == "msg" {

					//gzipReader, err := gzip.NewReader(bytes.NewReader(pac.Payload))

					//uncompressedData, err := io.ReadAll(gzipReader)
					uncompressedData, err := d.GzipUnzipReset(pac.Payload)
					if err != nil {
						log.Println("Gzip解压失败:", err)
						continue
					}

					response := &douyin.Response{}

					err = proto.Unmarshal(uncompressedData, response)
					if err != nil {
						log.Println("解析消息失败：", err)
						continue
					}

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
							log.Println("心跳包发送失败：", err)
							continue
						}
					}
					d.ProcessingMessage(response)
				}

			}
		}

	}
}

// Emit 匹配事件
func (d *DouyinLive) Emit(eventData *douyin.Message) {
	for _, handler := range d.eventHandlers {
		handler(eventData)
	}
}

// ProcessingMessage 处理消息
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

// Subscribe 订阅事件
func (d *DouyinLive) Subscribe(handler EventHandler) {
	d.eventHandlers = append(d.eventHandlers, handler)
}

// fttwid 获取ttwid
func (d *DouyinLive) fttwid() (string, error) {
	if d.ttwid != "" {
		return d.ttwid, nil
	}

	res, err := d.c.R().Get(d.liveurl)
	//
	if err != nil {
		return "", err
	}
	//
	var ttwidCookie http.Cookie
	for _, cookie := range res.Cookies() {
		//
		if cookie.Name == "ttwid" {

			ttwidCookie = *cookie
			d.ttwid = cookie.Value
			break
		}
	}
	return ttwidCookie.Value, nil
}

// froomid 获取room
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

// regroomid 正则获取roomid
func (d *DouyinLive) regroomid(s string) string {
	re := regexp.MustCompile(`roomId\\":\\"(\d+)\\"`)
	match := re.FindStringSubmatch(s)
	return match[1]
}

// regpushid 正则获取pushid
func (d *DouyinLive) regpushid(s string) string {
	re := regexp.MustCompile(`user_unique_id\\":\\"(\d+)\\"`)
	match := re.FindStringSubmatch(s)
	return match[1]
}
