package douyinlive

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jwwsjlm/douyinlive/generated/douyin"
	"github.com/jwwsjlm/douyinlive/generated/new_douyin"
	"github.com/jwwsjlm/douyinlive/jsScript"
	"github.com/jwwsjlm/douyinlive/utils"

	"github.com/gorilla/websocket"
	"github.com/imroc/req/v3"
	"github.com/spf13/cast"
	"google.golang.org/protobuf/proto"
)

// 正则表达式用于提取 roomID 和 pushID
var (
	roomIDRegexp = regexp.MustCompile(`roomId\\":\\"(\d+)\\"`)
	pushIDRegexp = regexp.MustCompile(`user_unique_id\\":\\"(\d+)\\"`)
	isLiveRegexp = regexp.MustCompile(`id_str\\":\\"(\d+)\\",\\"status\\":(\d+),\\"status_str\\":\\"(\d+)\\",\\"title\\":\\"(.*?)\\",\\"user_count_str\\":\\"(.*?)\\"`)
)

// DouyinLive 结构体表示一个抖音直播连接

// NewDouyinLive 创建一个新的 DouyinLive 实例
func NewDouyinLive(liveid string) (*DouyinLive, error) {
	ua := utils.RandomUserAgent()
	c := req.C().SetUserAgent(ua)
	d := &DouyinLive{
		liveid:        liveid,
		liveurl:       "https://live.douyin.com/",
		userAgent:     ua,
		c:             c,
		eventHandlers: make([]EventHandler, 0),
		headers:       http.Header{},
		buffers: &sync.Pool{
			New: func() interface{} {
				return &bytes.Buffer{}
			}},
	}

	// 获取 ttwid
	var err error
	d.ttwid, err = d.fetchTTWID()
	if err != nil {
		return nil, fmt.Errorf("获取 TTWID 失败: %w", err)
	}

	// 获取 roomid
	d.roomid, err = d.fetchRoomID()
	if err != nil {
		return nil, fmt.Errorf("获取 roomid 失败: %w", err)
	}
	// 加载 JavaScript 脚本
	err = jsScript.LoadGoja(d.userAgent)
	if err != nil {
		return nil, fmt.Errorf("加载 Goja 脚本失败: %w", err)
	}
	return d, nil
}

// fetchTTWID 获取 ttwid
func (d *DouyinLive) fetchTTWID() (string, error) {
	if d.ttwid != "" {
		return d.ttwid, nil
	}

	res, err := d.c.R().Get(d.liveurl)
	if err != nil {
		return "", fmt.Errorf("获取直播 URL 失败: %w", err)
	}

	for _, cookie := range res.Cookies() {
		if cookie.Name == "ttwid" {
			d.ttwid = cookie.Value
			return cookie.Value, nil
		}
	}
	return "", fmt.Errorf("未找到 ttwid cookie")
}
func (d *DouyinLive) getUrl(u string) (string, error) {
	//if d.roomid != "" {
	//	return d.roomid, nil
	//}
	t, _ := d.fetchTTWID()

	ttwid := &http.Cookie{
		Name:  "ttwid",
		Value: "ttwid=" + t + "&msToken=" + utils.GenerateMsToken(107),
	}
	acNonce := &http.Cookie{
		Name:  "__ac_nonce",
		Value: "0123407cc00a9e438deb4",
	}
	res, err := d.c.R().SetCookies(ttwid, acNonce).Get(u)
	if err != nil {
		log.Printf("获取房间 ID 失败: %v", err)
		return "", fmt.Errorf("获取房间 ID 失败: %w", err)
	}
	return res.String(), nil
}

// IsLive 是否直播
func (d *DouyinLive) IsLive() bool {
	result, err := d.getUrl(d.liveurl + d.liveid)
	if err != nil {
		d.isLiveClosed = false
		return false
	}
	str := extractMatch(isLiveRegexp, result, 2)
	log.Printf("直播状态: %v\n", str)
	d.isLiveClosed = str == "2"
	return str == "2"
}

// fetchRoomID 获取 roomID
func (d *DouyinLive) fetchRoomID() (string, error) {
	result, err := d.getUrl(d.liveurl + d.liveid)
	if err != nil {
		return result, fmt.Errorf("请求直播间信息失败: %w", err)
	}
	d.roomid = extractMatch(roomIDRegexp, result, 1)
	if d.roomid == "" {
		return "", errors.New("fetchRoomID: 未找到 roomID")
	}
	d.pushid = extractMatch(pushIDRegexp, result, 1)
	if d.pushid == "" {
		return "", errors.New("fetchRoomID: 未找到 pushid")
	}
	return d.roomid, nil
}

// extractMatch 从字符串中提取正则表达式匹配的内容
func extractMatch(re *regexp.Regexp, s string, i int) string {
	match := re.FindStringSubmatch(s)
	if len(match) > 1 {
		return match[i]
	}
	return ""
}

// GzipUnzipReset 使用 Reset 方法解压 gzip 数据
func (d *DouyinLive) GzipUnzipReset(compressedData []byte) ([]byte, error) {
	var err error
	buffer := d.buffers.Get().(*bytes.Buffer)
	defer func() {
		buffer.Reset()
		d.buffers.Put(buffer)
	}()

	_, err = buffer.Write(compressedData)
	if err != nil {
		return nil, err
	}

	if d.gzip != nil {
		err = d.gzip.Reset(buffer)
		if err != nil {
			err := d.gzip.Close()
			if err != nil {
				return nil, err
			}
			d.gzip = nil
			return nil, err
		}
	} else {
		d.gzip, err = gzip.NewReader(buffer)
		if err != nil {
			return nil, err
		}
	}

	defer func() {
		if d.gzip != nil {
			err := d.gzip.Close()
			if err != nil {
				return
			}
		}
	}()
	uncompressedBuffer := &bytes.Buffer{}
	_, err = io.Copy(uncompressedBuffer, d.gzip)
	if err != nil {
		return nil, err
	}

	return uncompressedBuffer.Bytes(), nil
}

// Start 开始连接和处理消息
func (d *DouyinLive) Start() {
	var err error
	d.wssurl = d.StitchUrl()
	d.headers.Add("user-agent", d.userAgent)
	d.headers.Add("cookie", fmt.Sprintf("ttwid=%s", d.ttwid))
	var response *http.Response
	if !d.IsLive() {
		log.Println("未开播,或者链接失败")
		return
	}

	d.Conn, response, err = websocket.DefaultDialer.Dial(d.wssurl, d.headers)
	if err != nil {
		log.Printf("链接失败: err:%v\nroomid:%v\n ttwid:%v\nwssurl:----%v\nresponse:%v\n", err, d.roomid, d.ttwid, d.wssurl, response)
		return
	}
	log.Println("链接成功")

	defer func() {
		if d.gzip != nil {
			err := d.gzip.Close()
			if err != nil {
				log.Println("gzip关闭失败:", err)
			} else {
				log.Println("gzip关闭")
			}
		}
		if d.Conn != nil {
			err = d.Conn.Close()
			if err != nil {
				log.Println("关闭ws链接失败", err)
			} else {
				log.Println("抖音ws链接关闭")
			}
		}
	}()
	var pbPac = &new_douyin.Webcast_Im_PushFrame{}
	var pbResp = &new_douyin.Webcast_Im_Response{}
	var pbAck = &new_douyin.Webcast_Im_PushFrame{}
	for d.isLiveClosed {
		messageType, message, err := d.Conn.ReadMessage()
		if err != nil {
			log.Println("读取消息失败-", err, message, messageType)
			if d.reconnect(5) {
				continue
			} else {
				break
			}
		} else {
			if message != nil {
				err := proto.Unmarshal(message, pbPac)
				if err != nil {
					log.Println("解析消息失败：", err)
					continue
				}
				n := utils.HasGzipEncoding(pbPac.Headers)
				if n && pbPac.PayloadType == "msg" {
					uncompressedData, err := d.GzipUnzipReset(pbPac.Payload)
					if err != nil {
						log.Println("Gzip解压失败:", err)
						continue
					}

					err = proto.Unmarshal(uncompressedData, pbResp)
					if err != nil {
						log.Println("解析消息失败：", err)
						continue
					}
					if pbResp.NeedAck {
						pbAck.Reset()
						pbAck.LogID = pbPac.LogID
						pbAck.PayloadType = "ack"
						pbAck.Payload = []byte(pbResp.InternalExt)

						serializedAck, err := proto.Marshal(pbAck)
						if err != nil {
							log.Println("proto心跳包序列化失败:", err)
							continue
						}
						err = d.Conn.WriteMessage(websocket.BinaryMessage, serializedAck)
						if err != nil {
							log.Println("心跳包发送失败：", err)
							continue
						}
					}
					d.ProcessingMessage(pbResp)
				}
			}
		}
	}
}

// reconnect 尝试重新连接
func (d *DouyinLive) reconnect(i int) bool {
	if d.Conn != nil {
		err := d.Conn.Close()
		if err != nil {
			return false
		}
		d.Conn = nil
	}
	var err error
	log.Println("尝试重新连接...")
	for attempt := 0; attempt < i; attempt++ {
		if d.Conn != nil {
			err := d.Conn.Close()
			if err != nil {
				log.Printf("关闭连接失败: %v", err)
			}
		}
		d.Conn, _, err = websocket.DefaultDialer.Dial(d.wssurl, d.headers)
		if err != nil {
			log.Printf("重连失败: %v", err)
			log.Printf("正在尝试第 %d 次重连...", attempt+1)
			time.Sleep(time.Duration(attempt) * time.Second)
		} else {
			log.Println("重连成功")
			return true
		}
	}
	log.Println("重连失败，退出程序")
	return false
}

// StitchUrl 构建 WebSocket 连接的 URL
func (d *DouyinLive) StitchUrl() string {
	smap := utils.NewOrderedMap(d.roomid, d.pushid)
	signaturemd5 := utils.GetxMSStub(smap)
	signature := jsScript.ExecuteJS(signaturemd5)
	browserInfo := strings.Split(d.userAgent, "Mozilla")[1]
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

// emit 触发事件处理器
func (d *DouyinLive) emit(eventData *new_douyin.Webcast_Im_Message) {
	for _, handler := range d.eventHandlers {
		handler(eventData)
	}
}

// ProcessingMessage 处理接收到的消息
func (d *DouyinLive) ProcessingMessage(response *new_douyin.Webcast_Im_Response) {
	for _, data := range response.Messages {
		d.emit(data)
		if data.Method == "WebcastControlMessage" {
			msg := &douyin.ControlMessage{}
			err := proto.Unmarshal(data.Payload, msg)
			if err != nil {
				log.Println("解析protobuf失败", err)
				return
			}
			if msg.Status == 3 {
				d.isLiveClosed = false
				log.Println("关闭ws链接成功")
			}
		}
	}
}

// Subscribe 订阅事件处理器
func (d *DouyinLive) Subscribe(handler EventHandler) {
	d.eventHandlers = append(d.eventHandlers, handler)
}
