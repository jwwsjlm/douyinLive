package douyinLive

import (
	"crypto/md5"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// ProtocolMode 标识请求使用的客户端协议画像。
// ProtocolMode identifies the client protocol profile used for requests.
type ProtocolMode string

const (
	// ProtocolModePC 复现抖音桌面客户端（awemePcClient）的协议特征。
	// ProtocolModePC reproduces the Douyin desktop client (awemePcClient) protocol.
	ProtocolModePC ProtocolMode = "pc"
	// ProtocolModeWeb 复现浏览器 Web 端的协议特征，即协议画像改造前的行为。
	// ProtocolModeWeb reproduces browser Web behavior, i.e. the pre-profile behavior.
	ProtocolModeWeb ProtocolMode = "web"
)

// DefaultProtocolMode 是未显式指定协议画像时使用的模式。
// DefaultProtocolMode is used when no protocol mode is specified.
// PC 画像仍处于测试阶段，默认保持兼容性更好的 Web 画像。
// The PC profile is experimental, so the compatibility-oriented Web profile remains default.
const DefaultProtocolMode = ProtocolModeWeb

// ErrProtocolModeInvalid 表示传入的协议画像名称无法识别。
// ErrProtocolModeInvalid reports an unrecognized protocol mode name.
var ErrProtocolModeInvalid = errors.New("协议画像必须是 pc 或 web")

// 抖音 PC 客户端（桌面版）协议常量。
// 数值全部来自 SunnyNet 抓包抖音桌面版 8.5.1：
// buildId 469669512，Chrome 136.0.7103.59，Electron 36.4.0-rs.31.release.pgo.7。
// Douyin PC desktop client protocol constants captured from desktop 8.5.1.
const (
	pcClientAppVersion    = "8.5.1"
	pcClientBuildID       = "469669512"
	pcClientChromeVersion = "136.0.7103.59"
	pcClientElectronBuild = "36.4.0-rs.31.release.pgo.7"
	pcClientDeviceOS      = "Windows 10"
	pcClientVendor        = "Microsoft Corporation"
	pcClientDeviceModel   = "Virtual Machine"
	// pcClientLiveVersion 对应抓包中的 __live_version__ Cookie 取值。
	pcClientLiveVersion = "1.1.5.5741"
)

// webProtocolPersistMsgCount 是 Web 端 WebSocket 查询串中的 need_persist_msg_count 取值。
// 抓包实测 PC 客户端使用 0，Web 端沿用历史行为 15。
// webProtocolPersistMsgCount is the Web-side need_persist_msg_count value; captures show the
// PC client sends 0 while the Web side keeps the historical 15.
const webProtocolPersistMsgCount = "15"

// pcProtocolPersistMsgCount 是 PC 客户端 WebSocket 查询串中的取值。
const pcProtocolPersistMsgCount = "0"

// cookieSeed 是协议画像固定携带的一个 Cookie。
// cookieSeed is one fixed cookie carried by a protocol profile.
type cookieSeed struct {
	Name  string
	Value string
}

// resolveProtocolMode 归一化并校验协议画像名称。
// resolveProtocolMode normalizes and validates a protocol mode name.
// 参数/Parameters:
//   - raw: 原始画像名称；空值取默认画像。 Raw mode name; empty selects the default.
func resolveProtocolMode(raw string) (ProtocolMode, error) {
	switch ProtocolMode(strings.ToLower(strings.TrimSpace(raw))) {
	case "":
		return DefaultProtocolMode, nil
	case ProtocolModePC:
		return ProtocolModePC, nil
	case ProtocolModeWeb:
		return ProtocolModeWeb, nil
	default:
		return "", fmt.Errorf("%w，得到 %q", ErrProtocolModeInvalid, raw)
	}
}

// userAgents 返回该协议画像可使用的 UA。
// userAgents returns the user agents available to this protocol mode.
func (mode ProtocolMode) userAgents() []string {
	if mode == ProtocolModeWeb {
		return webImpersonatedUserAgents
	}
	return []string{pcClientUserAgent()}
}

// clientHints 返回该协议画像发送的 sec-ch-ua* 请求头。
// clientHints returns the sec-ch-ua* headers sent by this protocol mode.
func (mode ProtocolMode) clientHints() map[string]string {
	if mode == ProtocolModeWeb {
		return webClientHintHeaders()
	}
	return pcClientClientHintHeaders()
}

// persistMsgCount 返回 WebSocket 查询串的 need_persist_msg_count。
// persistMsgCount returns the WebSocket need_persist_msg_count value.
func (mode ProtocolMode) persistMsgCount() string {
	if mode == ProtocolModeWeb {
		return webProtocolPersistMsgCount
	}
	return pcProtocolPersistMsgCount
}

// requestHeaders 返回该画像必须携带的专有请求头。
// requestHeaders returns the profile-specific request headers.
// 参数/Parameters:
//   - device: 会话级 PC 设备标识，仅 PC 画像使用。 Session PC device identity, used by the PC profile only.
func (mode ProtocolMode) requestHeaders(device pcClientDeviceIdentity) map[string]string {
	if mode != ProtocolModePC {
		return nil
	}
	return map[string]string{
		"X-AWEME-CLIENTVERSION":      pcClientAppVersion,
		"X-AWEME-DEVICEMANUFACTURER": device.Manufacturer,
		"X-AWEME-DEVICEMODEL":        device.DeviceModel,
		"X-AWEME-DEVICENAME":         device.DeviceName,
		"X-AWEME-DEVICEOS":           device.DeviceOS,
		"X-AWEME-GUID":               device.GUID,
	}
}

// cookieParts 返回该画像固定 Cookie 的 `name=value` 片段。
// cookieParts returns `name=value` fragments for the profile's fixed cookies.
// 参数/Parameters:
//   - existing: 已存在的 Cookie 名集合，避免覆盖服务端下发值。 Cookie names already present, so server-issued values win.
func (mode ProtocolMode) cookieParts(existing map[string]struct{}) []string {
	if mode != ProtocolModePC {
		return nil
	}
	parts := make([]string, 0, len(pcClientCookieSeeds))
	for _, seed := range pcClientCookieSeeds {
		if _, ok := existing[seed.Name]; ok {
			continue
		}
		parts = append(parts, seed.Name+"="+seed.Value)
	}
	return parts
}

// pcClientUserAgent 返回抖音桌面客户端的 User-Agent 原文。
// pcClientUserAgent returns the exact User-Agent sent by the Douyin desktop client.
func pcClientUserAgent() string {
	return fmt.Sprintf(
		"Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) douyin/%s Chrome/%s Electron/%s TTElectron/%s Safari/537.36 awemePcClient/%s buildId/%s osName/Windows",
		pcClientAppVersion,
		pcClientChromeVersion,
		pcClientElectronBuild,
		pcClientElectronBuild,
		pcClientAppVersion,
		pcClientBuildID,
	)
}

// pcClientClientHintHeaders 返回 Electron 客户端风格的 Client Hints。
// 抓包观测：客户端只声明 "Not.A/Brand" 与 "Chromium"，不含 "Google Chrome"。
// pcClientClientHintHeaders returns Electron-style Client Hints; the observed client
// advertises only "Not.A/Brand" and "Chromium", never "Google Chrome".
func pcClientClientHintHeaders() map[string]string {
	return map[string]string{
		"sec-ch-ua":          fmt.Sprintf(`"Not.A/Brand";v="99", "Chromium";v="%s"`, chromeMajorVersionFromUserAgent(pcClientUserAgent())),
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
	}
}

// webClientHintHeaders 返回通用 Chrome 风格的 Client Hints，与 Web 端 UA 主版本一致。
// webClientHintHeaders returns generic Chrome Client Hints matching the Web UA major version.
func webClientHintHeaders() map[string]string {
	return map[string]string{
		"sec-ch-ua": fmt.Sprintf(
			`"Not;A=Brand";v="8", "Chromium";v="%s", "Google Chrome";v="%s"`,
			httpImpersonationChromeMajor, httpImpersonationChromeMajor,
		),
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
	}
}

// webImpersonatedUserAgents 是 Web 画像的 UA 候选池。
// impersonatedUserAgents 必须与 req 当前 Chrome 133 TLS/HTTP2/HTTP3 画像保持同一主版本。
// webImpersonatedUserAgents is the Web profile UA pool, kept on the same major version as
// req's Chrome 133 TLS/HTTP2/HTTP3 profile.
var webImpersonatedUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + httpImpersonationChromeMajor + ".0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/" + httpImpersonationChromeMajor + ".0.0.0 Safari/537.36",
}

// pcClientDeviceIdentity 保存会话内稳定、跨会话轮换的 PC 客户端设备标识。
// pcClientDeviceIdentity holds PC-client device identifiers that stay stable within a session
// and rotate between sessions.
//
// 抓包中 X-AWEME-DEVICENAME 为 32 位十六进制、X-AWEME-GUID 为 128 位十六进制，
// 因此这里用 MD5 与 SHA-512 从会话种子派生，保证长度与字符集一致。
type pcClientDeviceIdentity struct {
	DeviceName   string
	GUID         string
	DeviceModel  string
	DeviceOS     string
	Manufacturer string
}

// newPCClientDeviceIdentity 从会话种子派生 PC 客户端设备标识。
// newPCClientDeviceIdentity derives PC-client device identifiers from a session seed.
// 参数/Parameters:
//   - seed: 会话级随机种子，同一会话内必须保持一致。 Session-scoped seed that must stay constant within a session.
func newPCClientDeviceIdentity(seed string) pcClientDeviceIdentity {
	nameDigest := md5.Sum([]byte("aweme-pc-device-name:" + seed))
	guidDigest := sha512.Sum512([]byte("aweme-pc-device-guid:" + seed))
	return pcClientDeviceIdentity{
		DeviceName:   hex.EncodeToString(nameDigest[:]),
		GUID:         hex.EncodeToString(guidDigest[:]),
		DeviceModel:  pcClientDeviceModel,
		DeviceOS:     pcClientDeviceOS,
		Manufacturer: pcClientVendor,
	}
}

// pcClientCookieSeeds 是抖音桌面客户端在直播页固定携带、且与登录态无关的 Cookie。
// 顺序与抓包观测一致，便于与真实客户端逐项比对。
var pcClientCookieSeeds = []cookieSeed{
	{"client_push", `{"live_push_record":""}`},
	{"enter_pc_once", "1"},
	{"__live_version__", `"` + pcClientLiveVersion + `"`},
	{"has_avx2", "true"},
	{"device_web_cpu_core", "10"},
	{"device_web_memory_size", "5"},
	{"is_support_rtm_web_ts", "0"},
	{"live_can_add_dy_2_desktop", `"0"`},
	{"live_use_vvc", `"true"`},
	{"home_can_add_dy_2_desktop", `"0"`},
	{"webcast_local_quality", "null"},
	{"is_dash_user", "1"},
	{"live_private_user", "0"},
}
