package douyinLive

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/jwwsjlm/douyinLive/v2/internal/webcastsign"
	"github.com/jwwsjlm/douyinLive/v2/jsScript"
	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
	"google.golang.org/protobuf/proto"
)

// TestDefaultProtocolModeUsesWeb 确认未配置时保持原有 Web 协议行为。
func TestDefaultProtocolModeUsesWeb(t *testing.T) {
	if DefaultProtocolMode != ProtocolModeWeb {
		t.Fatalf("DefaultProtocolMode = %q, want %q", DefaultProtocolMode, ProtocolModeWeb)
	}
	dl, err := newDouyinLive("1001", nil, "", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Dispose()
	if got := dl.ProtocolMode(); got != string(ProtocolModeWeb) {
		t.Fatalf("ProtocolMode() = %q, want web", got)
	}
}

// TestGojaFallbackSignsUnderPCUserAgent 确认 Goja 兜底签名在 PC 客户端 UA 下可用。
// webmssdk.js 在识别到 douyin/awemePcClient UA 时会走客户端分支，访问 console、
// PluginArray 等浏览器全局；Goja 环境脚本必须补齐这些全局，否则原生签名失败后的
// 兜底路径会直接 panic。
// TestGojaFallbackSignsUnderPCUserAgent verifies the Goja fallback signer works under the
// PC client UA, which drives webmssdk.js into a branch that touches browser globals.
func TestGojaFallbackSignsUnderPCUserAgent(t *testing.T) {
	const stub = "b0f5270c7225e9505d4dded773fed203"
	for _, mode := range []ProtocolMode{ProtocolModePC, ProtocolModeWeb} {
		t.Run(string(mode), func(t *testing.T) {
			signer, err := jsScript.NewSigner(mode.userAgents()[0], "ttwid=ttwid-value")
			if err != nil {
				t.Fatalf("NewSigner 失败: %v", err)
			}
			defer signer.Close()
			signature, err := signer.Sign(stub)
			if err != nil {
				t.Fatalf("Sign 失败: %v", err)
			}
			if len(signature) != 16 {
				t.Fatalf("签名 = %q (len=%d)，期望 16 字符", signature, len(signature))
			}
		})
	}
}

// 以下样本保留桌面客户端协议结构，但房间、设备、时间和签名均已脱敏替换。
const (
	probePCUserAgent = "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) douyin/8.5.1 Chrome/136.0.7103.59 Electron/36.4.0-rs.31.release.pgo.7 TTElectron/36.4.0-rs.31.release.pgo.7 Safari/537.36 awemePcClient/8.5.1 buildId/469669512 osName/Windows"
	probeRoomID      = "7000000000000000001"
	probeDeviceID    = "7000000000000000002"
	probeSignature   = "abcdefghijklmnop"
	probeFirstReqMS  = "1700000000000"
	probeFetchTime   = "1700000000100"
	probeCursorID    = "7000000000000000003"
	probeWRDSVersion = "7000000000000000004"

	// 客户端首帧：PushFrame{payload_type:"hb"}
	probeHeartbeatHex = "3a026862"
)

// TestProbeHeartbeatFrameMatchesPCClient 校验首帧字节与客户端完全一致。
func TestProbeHeartbeatFrameMatchesPCClient(t *testing.T) {
	data, err := buildHeartbeatFrame()
	if err != nil {
		t.Fatalf("构造心跳帧失败: %v", err)
	}
	got := hex.EncodeToString(data)
	t.Logf("项目心跳帧 = %s", got)
	t.Logf("客户端心跳帧 = %s", probeHeartbeatHex)
	if got != probeHeartbeatHex {
		t.Errorf("心跳帧与客户端不一致: got %s want %s", got, probeHeartbeatHex)
	}
}

// TestProbeSignatureShape 校验签名输入与输出形态。
func TestProbeSignatureShape(t *testing.T) {
	params := newWebsocketSignatureParams(probeRoomID, probeDeviceID)
	stub := params.XMSStub()
	t.Logf("签名输入 = %s", params.Joined())
	t.Logf("X-MS-STUB = %s", stub)
	wantStub := md5.Sum([]byte(params.Joined()))
	if want := hex.EncodeToString(wantStub[:]); stub != want {
		t.Errorf("X-MS-STUB = %s, want %s", stub, want)
	}

	generator := webcastsign.NewGenerator()
	signature, err := generator.Sign(stub)
	if err != nil {
		t.Fatalf("签名失败: %v", err)
	}
	t.Logf("本地签名输出 = %s (len=%d)", signature, len(signature))
	t.Logf("客户端签名样本 = %s (len=%d)", probeSignature, len(probeSignature))
	if len(signature) != len(probeSignature) {
		t.Errorf("签名长度不一致: got %d want %d", len(signature), len(probeSignature))
	}
}

// TestProbeAckFrameShape 校验 ACK 帧能够无损保存两种内部扩展载荷。
func TestProbeAckFrameShape(t *testing.T) {
	payloads := []string{
		"internal_src:pushserver|first_req_ms:" + probeFirstReqMS + "|seq:1|wss_msg_type:wrds|wrds_v:" + probeWRDSVersion,
		"internal_src:pushserver|first_req_ms:" + probeFirstReqMS + "|seq:1|wss_msg_type:r|wrds_v:" + probeWRDSVersion,
	}
	for index, payload := range payloads {
		encoded, err := proto.Marshal(&new_douyin.Webcast_Im_PushFrame{LogID: uint64(index + 1), PayloadType: "ack", Payload: []byte(payload)})
		if err != nil {
			t.Fatal(err)
		}
		var parsed new_douyin.Webcast_Im_PushFrame
		if err := proto.Unmarshal(encoded, &parsed); err != nil {
			t.Fatal(err)
		}
		if parsed.PayloadType != "ack" || string(parsed.Payload) != payload {
			t.Fatalf("ACK round-trip mismatch: payload_type=%q payload=%q", parsed.PayloadType, string(parsed.Payload))
		}
	}
}

// TestProbeQueryStringAgainstPCClient 逐字段比对 WS 查询串。
func TestProbeQueryStringAgainstPCClient(t *testing.T) {
	roomInfo := roomInfoSnapshot{roomID: probeRoomID, pushID: probeDeviceID}
	cursor := "d-1_u-1_h-1_t-" + probeFetchTime + "_r-" + probeCursorID
	internalExt := "internal_src:dim|wss_push_room_id:" + probeRoomID +
		"|wss_push_did:" + probeDeviceID +
		"|first_req_ms:" + probeFirstReqMS +
		"|fetch_time:" + probeFetchTime +
		"|seq:1|wss_info:0-" + probeFetchTime + "-0-0|wrds_v:" + probeWRDSVersion

	params := newWebsocketURLParamsWithScreen(roomInfo, probePCUserAgent, cursor, internalExt, probeSignature, 1366, 768)
	got := params.QueryString()

	gotFields := parseProbeQuery(got)
	for _, key := range clientQueryFieldOrder {
		if _, ok := gotFields[key]; !ok {
			t.Errorf("缺失字段 %s", key)
		}
	}
	wantValues := map[string]string{
		"browser_version":        websocketQueryValue(browserVersionFromUserAgent(probePCUserAgent)),
		"cursor":                 cursor,
		"internal_ext":           internalExt,
		"endpoint":               webcastEndpoint,
		"user_unique_id":         probeDeviceID,
		"need_persist_msg_count": pcProtocolPersistMsgCount,
		"room_id":                probeRoomID,
		"signature":              probeSignature,
	}
	for key, want := range wantValues {
		if got := gotFields[key]; got != want {
			t.Errorf("字段 %s = %q, want %q", key, got, want)
		}
	}
}

// TestProbeHandshakePingIntervalParsing 覆盖握手响应头 ping-interval 的解析。
func TestProbeHandshakePingIntervalParsing(t *testing.T) {
	cases := []struct {
		header string
		want   time.Duration
	}{
		{"ping-interval=15;", 15 * time.Second},
		{"Ping-Interval=15", 15 * time.Second},
		{" ping-interval = 15 ; ", 15 * time.Second},
		{"foo=bar;ping-interval=20;", 20 * time.Second},
		{"", 0},
		{"ping-interval=abc", 0},
		{"ping-interval=0", 0},
		{"ping-interval=-5", 0},
		{"no-interval=15", 0},
	}
	for _, tc := range cases {
		if got := parseHandshakePingInterval(tc.header); got != tc.want {
			t.Errorf("parseHandshakePingInterval(%q) = %v, want %v", tc.header, got, tc.want)
		}
	}
}

// clientQueryFieldOrder 是抓包中客户端字段的出现顺序。
var clientQueryFieldOrder = []string{
	"app_name", "version_code", "webcast_sdk_version", "update_version_code", "compress",
	"device_platform", "cookie_enabled", "screen_width", "screen_height", "browser_language",
	"browser_platform", "browser_name", "browser_version", "browser_online", "tz_name",
	"cursor", "internal_ext", "host", "aid", "live_id", "did_rule", "endpoint",
	"support_wrds", "user_unique_id", "im_path", "identity", "need_persist_msg_count",
	"insert_task_id", "live_reason", "room_id", "heartbeatDuration", "signature",
}

// parseProbeQuery 把查询串拆成键值表，保留原始值不做二次解码。
func parseProbeQuery(query string) map[string]string {
	fields := map[string]string{}
	for _, part := range strings.Split(query, "&") {
		if part == "" {
			continue
		}
		index := strings.Index(part, "=")
		if index < 0 {
			fields[part] = ""
			continue
		}
		fields[part[:index]] = part[index+1:]
	}
	return fields
}
