package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	douyinLive "github.com/jwwsjlm/douyinLive/v2"
	"go.yaml.in/yaml/v3"
)

// ErrVersionRequested tells the CLI layer that only version output was requested.
var ErrVersionRequested = errors.New("version requested")

const (
	signProviderLocal  = "local"
	signProviderTikHub = "tikhub"
	// defaultProtocolMode 与库默认协议画像保持一致：web 为稳定默认值，pc 仍为测试画像。
	defaultProtocolMode = "web"
)

// Keep Viper's former automatic search order so upgrades never silently skip
// an existing config file (and, in particular, its API key).
var automaticConfigNames = []string{
	"config.json", "config.toml", "config.yaml", "config.yml",
	"config.properties", "config.props", "config.prop", "config.hcl",
	"config.tfvars", "config.dotenv", "config.env", "config.ini", "config",
}

// CookieConfig stores the default cookie and per-room cookie overrides.
type CookieConfig struct {
	UseStored bool
	// useStoredSet distinguishes a programmatic zero-value Config from an explicit opt-out.
	useStoredSet bool
	Douyin       string
	Rooms        map[string]string
}

// ProxyConfig stores the default upstream proxy and fixed per-room overrides.
type ProxyConfig struct {
	URL   string            `yaml:"url"`
	Rooms map[string]string `yaml:"rooms"`
}

// ProtocolConfig selects the upstream protocol profile.
// ProtocolConfig 选择上游协议画像："pc" 复现抖音桌面客户端，"web" 复现浏览器 Web 端。
type ProtocolConfig struct {
	Mode string `yaml:"mode"`
}

// SetUseStoredCookie explicitly selects whether this configuration may use stored cookies.
func (c *CookieConfig) SetUseStoredCookie(enabled bool) {
	if c == nil {
		return
	}
	c.UseStored = enabled
	c.useStoredSet = true
}

type MonitorConfig struct {
	PollInterval   time.Duration
	NotifyInterval time.Duration
}

type LogConfig struct {
	Level string
}

type SignConfig struct {
	Provider string
}

type TikHubConfig struct {
	Key string
}

type APIConfig struct {
	Key            string
	AllowedDomains []string
}

type WebSocketConfig struct {
	Path           string
	AllowedOrigins []string
}

// Config stores all runtime configuration for the application.
type Config struct {
	Port      string
	Unknown   bool
	Cookie    CookieConfig
	Proxy     ProxyConfig
	Protocol  ProtocolConfig
	Monitor   MonitorConfig
	Log       LogConfig
	Sign      SignConfig
	TikHub    TikHubConfig
	API       APIConfig
	WebSocket WebSocketConfig
}

// configFileSchema mirrors the supported YAML keys so KnownFields can reject typos.
type configFileSchema struct {
	Port      string                    `yaml:"port"`
	Unknown   bool                      `yaml:"unknown"`
	Log       configFileLogSchema       `yaml:"log"`
	Sign      configFileSignSchema      `yaml:"sign"`
	TikHub    configFileTikHubSchema    `yaml:"tikhub"`
	API       configFileAPISchema       `yaml:"api"`
	WebSocket configFileWebSocketSchema `yaml:"websocket"`
	Monitor   configFileMonitorSchema   `yaml:"monitor"`
	Cookie    configFileCookieSchema    `yaml:"cookie"`
	Proxy     ProxyConfig               `yaml:"proxy"`
	Protocol  ProtocolConfig            `yaml:"protocol"`
}

type configFileLogSchema struct {
	Level string `yaml:"level"`
}

type configFileSignSchema struct {
	Provider string `yaml:"provider"`
}

type configFileTikHubSchema struct {
	Key string `yaml:"key"`
}

type configFileAPISchema struct {
	Key            string   `yaml:"key"`
	AllowedDomains []string `yaml:"allowed_domains"`
}

type configFileWebSocketSchema struct {
	Path           string   `yaml:"path"`
	AllowedOrigins []string `yaml:"allowed_origins"`
}

type configFileMonitorSchema struct {
	PollInterval   string `yaml:"poll_interval"`
	NotifyInterval string `yaml:"notify_interval"`
}

type configFileCookieSchema struct {
	UseStored bool              `yaml:"use_stored"`
	Douyin    string            `yaml:"douyin"`
	Rooms     map[string]string `yaml:"rooms"`
}

func defaultConfigFileSchema() configFileSchema {
	return configFileSchema{
		Port: "1088",
		Log:  configFileLogSchema{Level: "info"},
		Sign: configFileSignSchema{Provider: defaultSignProvider},
		API: configFileAPISchema{
			AllowedDomains: []string{"douyin.com"},
		},
		WebSocket: configFileWebSocketSchema{Path: "/ws"},
		Monitor: configFileMonitorSchema{
			PollInterval:   "15s",
			NotifyInterval: "30s",
		},
		Cookie: configFileCookieSchema{
			UseStored: true,
			Rooms:     map[string]string{},
		},
		Protocol: ProtocolConfig{Mode: defaultProtocolMode},
	}
}

func loadConfigFileSchema(path string) (configFileSchema, error) {
	schema := defaultConfigFileSchema()
	file, err := os.Open(path)
	if err != nil {
		return schema, err
	}
	defer file.Close()

	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&schema); err != nil {
		return schema, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return schema, errors.New("配置文件只能包含一个 YAML 文档")
		}
		return schema, err
	}
	return schema, nil
}

func findConfigFile(explicit string) (string, bool, error) {
	if explicit != "" {
		return explicit, true, nil
	}

	directories := make([]string, 0, 4)
	if executable, err := os.Executable(); err == nil {
		directories = append(directories, filepath.Dir(executable))
	}
	directories = append(directories, ".")
	if home, err := os.UserHomeDir(); err == nil {
		directories = append(directories, filepath.Join(home, ".app"))
	}
	directories = append(directories, filepath.Join(string(filepath.Separator), "etc", "app"))

	seen := make(map[string]struct{}, len(directories))
	for _, directory := range directories {
		directory = filepath.Clean(directory)
		if _, ok := seen[directory]; ok {
			continue
		}
		seen[directory] = struct{}{}
		for _, name := range automaticConfigNames {
			path := filepath.Join(directory, name)
			info, err := os.Stat(path)
			if err == nil && !info.IsDir() {
				return path, true, nil
			}
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return "", false, err
			}
		}
	}
	return "", false, nil
}

func envString(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok && value != "" {
		return value
	}
	return fallback
}

func envBool(name string, fallback bool) bool {
	raw, ok := os.LookupEnv(name)
	if !ok || raw == "" {
		return fallback
	}
	value, _ := strconv.ParseBool(raw) // Match Viper's previous false-on-invalid conversion.
	return value
}

// envStringSlice accepts JSON arrays and comma/whitespace-separated values.
func envStringSlice(name string, fallback []string) ([]string, error) {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return fallback, nil
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "[") {
		var values []string
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, fmt.Errorf("%s 必须是字符串数组、逗号分隔或空白分隔列表: %w", name, err)
		}
		return values, nil
	}
	return strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	}), nil
}

// envStringMap uses JSON so cookie/proxy punctuation remains unambiguous.
func envStringMap(name string, fallback map[string]string) (map[string]string, error) {
	raw, ok := os.LookupEnv(name)
	if !ok {
		return fallback, nil
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}, nil
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("%s 必须是 JSON 字符串对象", name)
	}
	if values == nil {
		return map[string]string{}, nil
	}
	return values, nil
}

func normalizeProxyConfig(config ProxyConfig) (ProxyConfig, error) {
	result := ProxyConfig{URL: strings.TrimSpace(config.URL), Rooms: make(map[string]string, len(config.Rooms))}
	if err := douyinLive.ValidateProxyURL(result.URL); err != nil {
		return ProxyConfig{}, fmt.Errorf("proxy.url 配置无效: %w", err)
	}
	for key, value := range config.Rooms {
		liveID, err := douyinLive.ValidateLiveID(strings.TrimSpace(key))
		if err != nil {
			return ProxyConfig{}, errors.New("proxy.rooms 包含无效直播间标识")
		}
		if _, exists := result.Rooms[liveID]; exists {
			return ProxyConfig{}, fmt.Errorf("proxy.rooms 包含重复直播间标识: %s", liveID)
		}
		if err := douyinLive.ValidateProxyURL(value); err != nil {
			return ProxyConfig{}, fmt.Errorf("proxy.rooms[%s] 配置无效: %w", liveID, err)
		}
		result.Rooms[liveID] = strings.TrimSpace(value)
	}
	return result, nil
}

func normalizeAllowedOrigins(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for index, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if value == "*" {
			return nil, fmt.Errorf("websocket.allowed_origins[%d] 不支持通配符 *；留空表示允许所有来源", index)
		}
		u, err := url.Parse(value)
		if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
			return nil, fmt.Errorf("websocket.allowed_origins[%d] 配置无效: %q", index, raw)
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "http" && scheme != "https" {
			return nil, fmt.Errorf("websocket.allowed_origins[%d] 仅支持 http/https Origin: %q", index, raw)
		}
		origin := scheme + "://" + strings.ToLower(u.Host)
		if _, ok := seen[origin]; ok {
			continue
		}
		seen[origin] = struct{}{}
		result = append(result, origin)
	}
	return result, nil
}

func normalizeAllowedDomains(values []string) ([]string, error) {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for index, value := range values {
		domain := strings.ToLower(strings.TrimSpace(value))
		domain = strings.TrimPrefix(domain, ".")
		domain = strings.TrimSuffix(domain, ".")
		if domain == "" {
			continue
		}
		if strings.ContainsAny(domain, "/?#\\\\@ :,\t\r\n") || strings.Contains(domain, "..") {
			return nil, fmt.Errorf("api.allowed_domains[%d] 配置无效: %q", index, value)
		}
		if domain != "douyin.com" && !strings.HasSuffix(domain, ".douyin.com") {
			return nil, fmt.Errorf("api.allowed_domains[%d] 只允许 douyin.com 及其子域名: %q", index, value)
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		result = append(result, domain)
	}
	if len(result) == 0 {
		return []string{"douyin.com"}, nil
	}
	return result, nil
}

func normalizeWebSocketPath(value string) (string, error) {
	path := strings.TrimSpace(value)
	if path == "" {
		path = "/ws"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		return "", errors.New("websocket.path 不能为根路径")
	}
	if strings.ContainsAny(path, "?#\\\t\r\n %{}") || strings.Contains(path, "//") || strings.Contains(path, "..") {
		return "", fmt.Errorf("websocket.path 配置无效: %q", value)
	}
	if path == "/health" || path == "/metrics" || path == "/api" || strings.HasPrefix(path, "/api/") {
		return "", fmt.Errorf("websocket.path 与保留 HTTP 路由冲突: %q", path)
	}
	return path, nil
}

// normalizeProtocolMode 校验并归一化上游协议画像配置。
// normalizeProtocolMode validates and normalizes the upstream protocol profile setting.
func normalizeProtocolMode(mode string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if normalized == "" {
		return defaultProtocolMode, nil
	}
	switch normalized {
	case string(douyinLive.ProtocolModePC), string(douyinLive.ProtocolModeWeb):
		return normalized, nil
	default:
		return "", fmt.Errorf("protocol.mode 配置无效: %s（可选 pc 或 web）", mode)
	}
}

func normalizeSignProvider(provider string) (string, error) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = defaultSignProvider
	}
	switch provider {
	case "local", "js", "javascript":
		return signProviderLocal, nil
	case "tikhub", "tik-hub", "tik_hub":
		return signProviderTikHub, nil
	default:
		return "", fmt.Errorf("sign.provider 配置无效: %s，可选值: local, tikhub", provider)
	}
}

// NewConfig applies defaults, YAML, environment variables, then explicit CLI flags.
func NewConfig() (*Config, error) {
	flags := flag.NewFlagSet("douyinLive", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	portFlag := flags.String("port", "1088", "WebSocket 服务端口")
	unknownFlag := flags.Bool("unknown", false, "是否输出未知源的 pb 消息")
	logLevelFlag := flags.String("log-level", "info", "日志级别: debug, info, warn, error")
	signProviderFlag := flags.String("sign-provider", defaultSignProvider, "WebSocket 签名来源: local, tikhub")
	tikHubKeyFlag := flags.String("tikhub-key", "", "TikHub API Key，用于在线生成 WebSocket 签名")
	proxyURLFlag := flags.String("proxy-url", "", "抖音采集默认代理 URL，支持 http 或 socks5")
	protocolFlag := flags.String("protocol", defaultProtocolMode, "上游协议画像: web（默认，浏览器 Web 端）, pc（测试，抖音桌面客户端）")
	configFileFlag := flags.String("config", "", "指定配置文件路径")
	versionFlag := flags.Bool("version", false, "输出版本信息")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return nil, err
	}
	if *versionFlag {
		return nil, ErrVersionRequested
	}
	changed := make(map[string]bool)
	flags.Visit(func(value *flag.Flag) { changed[value.Name] = true })

	configPath, loaded, err := findConfigFile(*configFileFlag)
	if err != nil {
		return nil, fmt.Errorf("查找配置文件失败：%w", err)
	}
	schema := defaultConfigFileSchema()
	if loaded {
		schema, err = loadConfigFileSchema(configPath)
		if err != nil {
			return nil, fmt.Errorf("读取配置文件失败：%w", err)
		}
		fmt.Printf("✅ 使用配置文件：%s\n", configPath)
	} else {
		fmt.Println("⚠️  配置文件未找到，使用默认值或命令行参数")
		fmt.Println("💡 建议在同目录下创建 config.yaml 文件")
	}

	schema.Port = envString("APP_PORT", schema.Port)
	schema.Log.Level = envString("APP_LOG_LEVEL", schema.Log.Level)
	schema.Sign.Provider = envString("APP_SIGN_PROVIDER", schema.Sign.Provider)
	schema.TikHub.Key = envString("APP_TIKHUB_KEY", schema.TikHub.Key)
	schema.API.Key = envString("APP_API_KEY", schema.API.Key)
	schema.WebSocket.Path = envString("APP_WEBSOCKET_PATH", schema.WebSocket.Path)
	schema.Cookie.Douyin = envString("APP_COOKIE_DOUYIN", schema.Cookie.Douyin)
	schema.Proxy.URL = envString("APP_PROXY_URL", schema.Proxy.URL)
	schema.Protocol.Mode = envString("APP_PROTOCOL", schema.Protocol.Mode)
	schema.Monitor.PollInterval = envString("APP_MONITOR_POLL_INTERVAL", schema.Monitor.PollInterval)
	schema.Monitor.NotifyInterval = envString("APP_MONITOR_NOTIFY_INTERVAL", schema.Monitor.NotifyInterval)
	schema.Unknown = envBool("APP_UNKNOWN", schema.Unknown)
	schema.Cookie.UseStored = envBool("APP_COOKIE_USE_STORED", schema.Cookie.UseStored)
	if schema.API.AllowedDomains, err = envStringSlice("APP_API_ALLOWED_DOMAINS", schema.API.AllowedDomains); err != nil {
		return nil, err
	}
	if schema.WebSocket.AllowedOrigins, err = envStringSlice("APP_WEBSOCKET_ALLOWED_ORIGINS", schema.WebSocket.AllowedOrigins); err != nil {
		return nil, err
	}
	if schema.Cookie.Rooms, err = envStringMap("APP_COOKIE_ROOMS", schema.Cookie.Rooms); err != nil {
		return nil, err
	}
	if schema.Proxy.Rooms, err = envStringMap("APP_PROXY_ROOMS", schema.Proxy.Rooms); err != nil {
		return nil, err
	}

	if changed["port"] {
		schema.Port = *portFlag
	}
	if changed["unknown"] {
		schema.Unknown = *unknownFlag
	}
	if changed["log-level"] {
		schema.Log.Level = *logLevelFlag
	}
	if changed["sign-provider"] {
		schema.Sign.Provider = *signProviderFlag
	}
	if changed["tikhub-key"] {
		schema.TikHub.Key = *tikHubKeyFlag
	}
	if changed["proxy-url"] {
		schema.Proxy.URL = *proxyURLFlag
	}
	if changed["protocol"] {
		schema.Protocol.Mode = *protocolFlag
	}
	proxyConfig, err := normalizeProxyConfig(schema.Proxy)
	if err != nil {
		return nil, err
	}
	protocolMode, err := normalizeProtocolMode(schema.Protocol.Mode)
	if err != nil {
		return nil, err
	}

	if _, err := parseConfiguredPort(schema.Port); err != nil {
		return nil, err
	}
	pollInterval, err := time.ParseDuration(schema.Monitor.PollInterval)
	if err != nil {
		return nil, fmt.Errorf("monitor.poll_interval 配置无效：%w", err)
	}
	if pollInterval <= 0 {
		return nil, errors.New("monitor.poll_interval 必须大于 0")
	}
	notifyInterval, err := time.ParseDuration(schema.Monitor.NotifyInterval)
	if err != nil {
		return nil, fmt.Errorf("monitor.notify_interval 配置无效：%w", err)
	}
	if notifyInterval <= 0 {
		return nil, errors.New("monitor.notify_interval 必须大于 0")
	}

	logLevel := strings.ToLower(strings.TrimSpace(schema.Log.Level))
	if logLevel == "" {
		logLevel = "info"
	}
	switch logLevel {
	case "debug", "info", "warn", "error":
	default:
		return nil, fmt.Errorf("log.level 配置无效: %s", logLevel)
	}

	signProvider, err := normalizeSignProvider(schema.Sign.Provider)
	if err != nil {
		return nil, err
	}
	tikHubKey := strings.TrimSpace(schema.TikHub.Key)
	if signProvider == signProviderTikHub && tikHubKey == "" {
		return nil, errors.New("sign.provider=tikhub 时必须配置 tikhub.key、APP_TIKHUB_KEY 或 --tikhub-key")
	}
	websocketPath, err := normalizeWebSocketPath(schema.WebSocket.Path)
	if err != nil {
		return nil, err
	}
	allowedOrigins, err := normalizeAllowedOrigins(schema.WebSocket.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	allowedDomains, err := normalizeAllowedDomains(schema.API.AllowedDomains)
	if err != nil {
		return nil, err
	}
	douyinCookie := strings.TrimSpace(schema.Cookie.Douyin)
	if err := validateCookieOverride(douyinCookie); err != nil {
		return nil, fmt.Errorf("cookie.douyin 配置无效: %w", err)
	}
	normalizedRoomCookies := make(map[string]string, len(schema.Cookie.Rooms))
	for roomID, rawCookie := range schema.Cookie.Rooms {
		roomID = strings.TrimSpace(roomID)
		if _, err := douyinLive.ValidateLiveID(roomID); err != nil {
			return nil, fmt.Errorf("cookie.rooms 中包含无效直播间标识: %q", roomID)
		}
		cookie := strings.TrimSpace(rawCookie)
		if err := validateCookieOverride(cookie); err != nil {
			return nil, fmt.Errorf("cookie.rooms[%q] 配置无效: %w", roomID, err)
		}
		normalizedRoomCookies[roomID] = cookie
	}

	return &Config{
		Proxy:    proxyConfig,
		Protocol: ProtocolConfig{Mode: protocolMode},
		Port:     schema.Port,
		Unknown:  schema.Unknown,
		Cookie: CookieConfig{
			UseStored:    schema.Cookie.UseStored,
			useStoredSet: true,
			Douyin:       douyinCookie,
			Rooms:        normalizedRoomCookies,
		},
		Monitor: MonitorConfig{PollInterval: pollInterval, NotifyInterval: notifyInterval},
		Log:     LogConfig{Level: logLevel},
		Sign:    SignConfig{Provider: signProvider},
		TikHub:  TikHubConfig{Key: tikHubKey},
		API:     APIConfig{Key: strings.TrimSpace(schema.API.Key), AllowedDomains: allowedDomains},
		WebSocket: WebSocketConfig{
			Path:           websocketPath,
			AllowedOrigins: allowedOrigins,
		},
	}, nil
}
