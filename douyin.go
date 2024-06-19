package douyinlive

import (
	"DouyinLive/generated/douyin"
	"DouyinLive/utils"
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/imroc/req/v3"
	"google.golang.org/protobuf/proto"
	"io"
	"log"
	"net/http"
	"regexp"
)

func NewDouyinLive(liveid string) (*DouyinLive, error) {

	c := req.C().SetUserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko)")
	d := &DouyinLive{
		ttwid:         "",
		roomid:        "",
		liveid:        liveid,
		liveurl:       "https://live.douyin.com/",
		useragent:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko)",
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

func (d *DouyinLive) Start() {

	//d.roomid = d.froomid()
	//d.ttwid, _ = d.fttwid()
	wssURL := "wss://webcast3-ws-web-lq.douyin.com/webcast/im/push/v2/?app_name=douyin_web&version_code=180800&webcast_sdk_version=1.3.0&update_version_code=1.3.0&compress=gzip&internal_ext=internal_src:dim|wss_push_room_id:" + d.roomid + "|wss_push_did:" + d.roomid + "|dim_log_id:202302171547011A160A7BAA76660E13ED|fetch_time:1676620021641|seq:1|wss_info:0-1676620021641-0-0|wrds_kvs:WebcastRoomStatsMessage-1676620020691146024_WebcastRoomRankMessage-1676619972726895075_AudienceGiftSyncData-1676619980834317696_HighlightContainerSyncData-2&cursor=t-1676620021641_r-1_d-1_u-1_h-1&host=https://live.douyin.com&aid=6383&live_id=1&did_rule=3&debug=false&endpoint=live_pc&support_wrds=1&im_path=/webcast/im/fetch/&user_unique_id=" + d.roomid + "&device_platform=web&cookie_enabled=true&screen_width=1440&screen_height=900&browser_language=zh&browser_platform=MacIntel&browser_name=Mozilla&browser_version=5.0%20(Macintosh;%20Intel%20Mac%20OS%20X%2010_15_7)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/110.0.0.0%20Safari/537.36&browser_online=true&tz_name=Asia/Shanghai&identity=audience&room_id=" + d.roomid + "&heartbeatDuration=0&signature=00000000"
	//t, _ := d.fttwid()

	headers := http.Header{
		"Cookie":     []string{fmt.Sprintf("ttwid=%s", d.ttwid)},
		"user-agent": []string{d.useragent},
	}
	//fmt.Printf("协议头%v,%v\n", headers, d.useragent)
	//return
	var err error

	d.Conn, _, err = websocket.DefaultDialer.Dial(wssURL, headers)
	//fmt.Println(wssURL)
	if err != nil {
		log.Printf("链接失败: err:%v\nroomid:%v\n ttwid:%v\n", err, d.roomid, d.ttwid)
		return

	}
	//d.Conn.Close()
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
	d.roomid = match[1]
	return match[1]

}
