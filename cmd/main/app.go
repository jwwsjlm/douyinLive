package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
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
	// maxAutomaticPortAttempts 限制端口占用时自动顺延的次数，避免配置错误导致无限循环。
	// maxAutomaticPortAttempts bounds automatic port fallback to prevent endless loops.
	maxAutomaticPortAttempts = 100
)

type tcpListenFunc func(network, address string) (net.Listener, error)

// App 封装 HTTP 服务、房间管理器和运行配置。
// App bundles the HTTP server, room manager, and runtime configuration.
type App struct {
	ctx         context.Context
	logger      *appLogger
	config      *Config
	roomManager *RoomManager
	httpServer  *http.Server
	runningPort string
	ready       chan struct{}
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
	if logger == nil {
		logger = newAppLogger(nil)
	}

	roomManager := NewRoomManager(
		logger,
		config.Unknown,
		config.Cookie.Douyin,
		config.Cookie.Rooms,
		config.Sign.Provider,
		config.TikHub.Key,
		config.Monitor.PollInterval,
		config.Monitor.NotifyInterval,
	)
	return &App{
		ctx:         ctx,
		logger:      logger,
		config:      config,
		roomManager: roomManager,
		ready:       make(chan struct{}),
	}, nil
}

// Run 启动 WebSocket HTTP 服务，并在端口占用时自动尝试下一个端口。
// Run starts the WebSocket HTTP server and tries the next port when the configured one is busy.
func (a *App) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/", a.handleWebSocket)
	mux.HandleFunc("/healthz", a.handleHealth)

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

	close(a.ready)
	a.logger.Info("WebSocket 服务监听中", "port", a.runningPort)
	return a.httpServer.Serve(listener)
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

// handleHealth 返回进程健康状态和当前构建版本。
// handleHealth returns process health and current build metadata.
func (a *App) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"version": VersionString(),
	})
}

// Shutdown 优雅关闭房间管理器和 HTTP 服务。
// Shutdown gracefully stops the room manager and HTTP server.
func (a *App) Shutdown() error {
	a.logger.Info("正在关闭 RoomManager")
	a.roomManager.CloseAll()

	if a.httpServer != nil {
		a.logger.Info("正在关闭 HTTP 服务")
		shutdownCtx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
		defer cancel()
		return a.httpServer.Shutdown(shutdownCtx)
	}
	return nil
}

// handleWebSocket 解析房间 ID 并升级客户端 WebSocket 连接。
// handleWebSocket parses the room ID and upgrades the client WebSocket connection.
// 参数/Parameters:
//   - w: HTTP 响应写入器。 HTTP response writer.
//   - r: 包含房间 ID 和可选 Cookie 覆盖的 HTTP 请求。 HTTP request carrying room ID and optional cookie override.
func (a *App) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	roomID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/ws/"), "/")
	if roomID == "" {
		http.Error(w, "无效的房间ID", http.StatusBadRequest)
		return
	}

	cookieOverride, err := parseCookieOverride(r)
	if err != nil {
		http.Error(w, "Cookie 参数无效", http.StatusBadRequest)
		return
	}

	a.logger.Info("接收到 WebSocket 连接请求", "room_id", roomID, "remote_addr", r.RemoteAddr)

	room := a.roomManager.GetOrCreateRoom(roomID, cookieOverride)
	handler := NewWsHandler(room)

	upgrader := gws.NewUpgrader(handler, &gws.ServerOption{
		ParallelEnabled:   true,
		Recovery:          gws.Recovery,
		PermessageDeflate: gws.PermessageDeflate{Enabled: true},
	})

	socket, err := upgrader.Upgrade(w, r)
	if err != nil {
		a.logger.Warn("升级 WebSocket 失败", "room_id", roomID, "remote_addr", r.RemoteAddr, "err", err)
		return
	}

	go socket.ReadLoop()
}

// parseCookieOverride 从查询参数读取本次连接专用 Cookie。
// parseCookieOverride reads the per-connection cookie override from query parameters.
// 参数/Parameters:
//   - r: 当前 HTTP 请求。 Current HTTP request.
func parseCookieOverride(r *http.Request) (string, error) {
	q := r.URL.Query()
	if cookie := strings.TrimSpace(q.Get("cookie")); cookie != "" {
		return cookie, nil
	}

	cookieB64 := strings.TrimSpace(q.Get("cookie_b64"))
	if cookieB64 == "" {
		return "", nil
	}

	cookie, err := decodeCookieBase64(cookieB64)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(cookie), nil
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
