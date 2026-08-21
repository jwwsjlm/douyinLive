package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lxzan/gws"
)

const (
	// httpReadHeaderTimeout 限制读取请求头的最长时间。
	// httpReadHeaderTimeout limits how long the server waits for request headers.
	httpReadHeaderTimeout = 5 * time.Second
	// httpIdleTimeout 限制空闲 HTTP 连接保留时间。
	// httpIdleTimeout limits how long idle HTTP connections are kept.
	httpIdleTimeout = 2 * time.Minute
	// httpMaxHeaderBytes 限制 HTTP 请求头最大字节数。
	// httpMaxHeaderBytes limits the maximum HTTP request header size.
	httpMaxHeaderBytes = 16 << 10
	// downstreamReadMaxPayloadSize limits client messages because the local
	// WebSocket protocol only accepts the small text heartbeat "ping".
	// downstreamReadMaxPayloadSize 限制下游客户端消息；本地协议只接受很小的文本心跳 "ping"。
	downstreamReadMaxPayloadSize = 4 << 10
	// downstreamHandshakeTimeout bounds the local WebSocket upgrade handshake.
	// downstreamHandshakeTimeout 限制本地 WebSocket 升级握手耗时。
	downstreamHandshakeTimeout = 5 * time.Second
	// maxAutomaticPortAttempts 限制端口占用时自动顺延的次数，避免配置错误导致无限循环。
	// maxAutomaticPortAttempts bounds automatic port fallback to prevent endless loops.
	maxAutomaticPortAttempts = 100
)

type tcpListenFunc func(network, address string) (net.Listener, error)

// App 封装 HTTP 服务、房间管理器和运行配置。
// App bundles the HTTP server, room manager, and runtime configuration.
type App struct {
	ctx          context.Context
	logger       *appLogger
	config       *Config
	roomManager  *RoomManager
	httpServer   *http.Server
	runningPort  string
	ready        chan struct{}
	metrics      *apiMetrics
	shutdownOnce sync.Once
	shutdownErr  error
}

// NewApp 创建应用实例并初始化房间管理器。
// NewApp creates an application instance and initializes the room manager.
// 参数/Parameters:
//   - ctx: 应用生命周期上下文。 Application lifecycle context.
//   - config: 已加载的运行配置。 Loaded runtime configuration.
//   - logger: 应用日志器。 Application logger.
func NewApp(ctx context.Context, config *Config, logger *appLogger) (*App, error) {
	if config == nil {
		return nil, errors.New("config 不能为空")
	}
	// Keep the caller-owned configuration immutable after application creation.
	// 应用创建后不再修改调用方持有的配置对象，避免共享配置产生数据竞争。
	configCopy := *config
	if config.Cookie.Rooms != nil {
		configCopy.Cookie.Rooms = make(map[string]string, len(config.Cookie.Rooms))
		for roomID, cookie := range config.Cookie.Rooms {
			configCopy.Cookie.Rooms[roomID] = cookie
		}
	}
	configCopy.WebSocket.AllowedOrigins = append([]string(nil), config.WebSocket.AllowedOrigins...)
	configCopy.API.AllowedDomains = append([]string(nil), config.API.AllowedDomains...)
	config = &configCopy
	websocketPath, err := normalizeWebSocketPath(config.WebSocket.Path)
	if err != nil {
		return nil, err
	}
	allowedOrigins, err := normalizeAllowedOrigins(config.WebSocket.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	allowedDomains, err := normalizeAllowedDomains(config.API.AllowedDomains)
	if err != nil {
		return nil, err
	}
	config.WebSocket.Path = websocketPath
	config.WebSocket.AllowedOrigins = allowedOrigins
	config.API.AllowedDomains = allowedDomains
	if logger == nil {
		logger = newAppLogger(nil)
	}
	if ctx == nil {
		ctx = context.Background()
	}

	roomManager := NewRoomManagerWithOptions(RoomManagerOptions{
		Logger: logger, Unknown: config.Unknown, Cookie: config.Cookie.Douyin,
		RoomCookies: config.Cookie.Rooms, SignProvider: config.Sign.Provider,
		TikHubKey: config.TikHub.Key, PollInterval: config.Monitor.PollInterval,
		NotifyInterval: config.Monitor.NotifyInterval, UseStoredCookie: boolPtr(config.Cookie.UseStored),
	})
	metrics := newAPIMetrics()
	roomManager.metrics = metrics
	return &App{
		ctx:         ctx,
		logger:      logger,
		config:      config,
		roomManager: roomManager,
		ready:       make(chan struct{}),
		metrics:     metrics,
	}, nil
}

// Run 启动 WebSocket HTTP 服务，并在端口占用时自动尝试下一个端口。
// Run starts the WebSocket HTTP server and tries the next port when the configured one is busy.
func (a *App) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc(a.websocketRoutePrefix(), a.handleWebSocket)
	a.registerHTTPAPI(mux)

	port, err := parseConfiguredPort(a.config.Port)
	if err != nil {
		return err
	}

	listener, selectedPort, err := listenOnAvailablePort(port, net.Listen)
	if err != nil {
		return err
	}
	addr := ":" + strconv.Itoa(selectedPort)
	a.httpServer = newHTTPServer(addr, mux)
	a.runningPort = strconv.Itoa(selectedPort)
	serverDone := make(chan struct{})
	defer close(serverDone)
	if a.ctx != nil && a.ctx.Done() != nil {
		go func() {
			select {
			case <-a.ctx.Done():
				_ = a.Shutdown()
			case <-serverDone:
			}
		}()
	}

	close(a.ready)
	a.logger.Info("WebSocket 服务监听中", "port", a.runningPort)
	return a.httpServer.Serve(listener)
}

func (a *App) websocketRoutePrefix() string {
	if a == nil || a.config == nil {
		return "/ws/"
	}
	return a.config.WebSocket.Path + "/"
}

// parseConfiguredPort 校验配置端口并转换为整数。
// parseConfiguredPort validates and parses the configured TCP port.
func parseConfiguredPort(value string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, fmt.Errorf("port 配置无效 %q: %w", value, err)
	}
	if port < 1 || port > 65535 {
		return 0, fmt.Errorf("port 必须在 1 到 65535 之间: %d", port)
	}
	return port, nil
}

// listenOnAvailablePort 监听指定端口；仅在端口占用时有限次数地尝试后续端口。
// listenOnAvailablePort listens on the requested port and only falls forward for address-in-use errors.
func listenOnAvailablePort(startPort int, listen tcpListenFunc) (net.Listener, int, error) {
	if listen == nil {
		return nil, 0, errors.New("listen function 不能为空")
	}
	lastPort := min(65535, startPort+maxAutomaticPortAttempts-1)
	for port := startPort; port <= lastPort; port++ {
		addr := ":" + strconv.Itoa(port)
		listener, err := listen("tcp", addr)
		if err == nil {
			return listener, port, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, 0, fmt.Errorf("监听 %s 失败: %w", addr, err)
		}
	}
	return nil, 0, fmt.Errorf("端口 %d-%d 均已被占用", startPort, lastPort)
}

// newHTTPServer 创建带基础超时限制的 HTTP 服务。
// newHTTPServer creates an HTTP server with basic timeout limits.
// 参数/Parameters:
//   - addr: HTTP 服务监听地址。 HTTP server listen address.
//   - handler: HTTP 请求处理器。 HTTP request handler.
func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		IdleTimeout:       httpIdleTimeout,
		MaxHeaderBytes:    httpMaxHeaderBytes,
	}
}

// Shutdown 优雅关闭房间管理器和 HTTP 服务。
// Shutdown gracefully stops the room manager and HTTP server.
func (a *App) Shutdown() error {
	if a == nil {
		return nil
	}
	a.shutdownOnce.Do(func() {
		a.logger.Info("正在关闭 RoomManager")
		if a.roomManager != nil {
			a.roomManager.Close()
		}

		if a.httpServer != nil {
			a.logger.Info("正在关闭 HTTP 服务")
			// Shutdown must not inherit an already-cancelled application context.
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			a.shutdownErr = a.httpServer.Shutdown(shutdownCtx)
		}
	})
	return a.shutdownErr
}

// handleWebSocket 解析房间 ID 并升级客户端 WebSocket 连接。
// handleWebSocket parses the room ID and upgrades the client WebSocket connection.
// 参数/Parameters:
//   - w: HTTP 响应写入器。 HTTP response writer.
//   - r: 包含房间 ID 和可选 Cookie 覆盖的 HTTP 请求。 HTTP request carrying room ID and optional cookie override.
func (a *App) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	requestID := requestIDForRequest(r)
	w.Header().Set("X-Request-ID", requestID)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "仅支持 GET 请求", http.StatusMethodNotAllowed)
		return
	}
	if !a.authorizeAPI(r) {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"douyinlive\"")
		http.Error(w, "缺少或无效的 API Key", http.StatusUnauthorized)
		return
	}
	if !a.authorizeWebSocketOrigin(r) {
		http.Error(w, "WebSocket Origin 不被允许", http.StatusForbidden)
		return
	}
	roomID, err := parseLiveIDPath(r.URL.Path, a.websocketRoutePrefix())
	if err != nil {
		http.Error(w, "无效的房间ID", http.StatusBadRequest)
		return
	}

	cookieOverride, err := parseCookieOverride(r)
	if err != nil {
		http.Error(w, "Cookie 参数无效", http.StatusBadRequest)
		return
	}

	a.logger.Info("接收到 WebSocket 连接请求", "room_id", roomID, "remote_addr", r.RemoteAddr)

	room := a.roomManager.AcquireRoom(roomID, cookieOverride)
	if room == nil {
		http.Error(w, "服务正在关闭", http.StatusServiceUnavailable)
		return
	}
	handler := NewWsHandler(room)

	upgrader := gws.NewUpgrader(handler, downstreamServerOption())

	socket, err := upgrader.Upgrade(w, r)
	if err != nil {
		room.releaseClientReservation()
		a.logger.Warn("升级 WebSocket 失败", "room_id", roomID, "remote_addr", r.RemoteAddr, "err", err)
		return
	}

	go socket.ReadLoop()
}

func downstreamServerOption() *gws.ServerOption {
	return &gws.ServerOption{
		ParallelEnabled:     false,
		Recovery:            gws.Recovery,
		PermessageDeflate:   gws.PermessageDeflate{Enabled: true},
		ReadMaxPayloadSize:  downstreamReadMaxPayloadSize,
		WriteMaxPayloadSize: maxClientMessageBytes,
		HandshakeTimeout:    downstreamHandshakeTimeout,
		CheckUtf8Enabled:    true,
	}
}

// authorizeWebSocketOrigin checks the optional browser Origin allowlist.
// authorizeWebSocketOrigin 校验可选的浏览器 Origin 白名单；未配置时保持兼容并允许所有来源。
func (a *App) authorizeWebSocketOrigin(r *http.Request) bool {
	allowed := a.config.WebSocket.AllowedOrigins
	if len(allowed) == 0 {
		return true
	}
	raw := strings.TrimSpace(r.Header.Get("Origin"))
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	origin := strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
	for _, candidate := range allowed {
		if strings.EqualFold(origin, candidate) {
			return true
		}
	}
	return false
}

// parseCookieOverride 从查询参数读取本次连接专用 Cookie。
// parseCookieOverride reads the per-connection cookie override from query parameters.
// 参数/Parameters:
//   - r: 当前 HTTP 请求。 Current HTTP request.
func parseCookieOverride(r *http.Request) (string, error) {
	if r == nil || r.URL == nil {
		return "", nil
	}
	q := r.URL.Query()
	rawCookie := strings.TrimSpace(q.Get("cookie"))
	rawCookieB64 := strings.TrimSpace(q.Get("cookie_b64"))
	if rawCookie != "" && rawCookieB64 != "" {
		return "", fmt.Errorf("cookie 和 cookie_b64 不能同时使用")
	}
	if rawCookie != "" {
		if err := validateCookieOverride(rawCookie); err != nil {
			return "", err
		}
		return rawCookie, nil
	}

	if rawCookieB64 == "" {
		return "", nil
	}

	cookie, err := decodeCookieBase64(rawCookieB64)
	if err != nil {
		return "", err
	}
	cookie = strings.TrimSpace(cookie)
	if err := validateCookieOverride(cookie); err != nil {
		return "", err
	}
	return cookie, nil
}

const maxCookieOverrideBytes = 16 << 10

func validateCookieOverride(cookie string) error {
	if cookie == "" {
		return nil
	}
	if len(cookie) > maxCookieOverrideBytes {
		return fmt.Errorf("cookie 参数过长")
	}
	for _, ch := range cookie {
		if ch < 0x20 || ch == 0x7f {
			return fmt.Errorf("cookie 参数包含非法控制字符")
		}
	}
	return nil
}

// decodeCookieBase64 解码 URL 安全或标准 Base64 Cookie。
// decodeCookieBase64 decodes URL-safe or standard Base64 cookie values.
// 参数/Parameters:
//   - value: Base64 编码后的 Cookie 文本。 Base64-encoded cookie text.
func decodeCookieBase64(value string) (string, error) {
	decoders := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.URLEncoding,
		base64.StdEncoding,
	}
	for _, decoder := range decoders {
		data, err := decoder.DecodeString(value)
		if err == nil {
			return string(data), nil
		}
	}
	return "", fmt.Errorf("invalid base64 cookie")
}
