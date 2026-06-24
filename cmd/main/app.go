package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lxzan/gws"
)

const (
	httpReadHeaderTimeout = 5 * time.Second
	httpIdleTimeout       = 2 * time.Minute
	httpMaxHeaderBytes    = 16 << 10
)

// App 是应用的核心结构体，封装了所有依赖
type App struct {
	ctx         context.Context
	logger      *appLogger
	config      *Config
	roomManager *RoomManager
	httpServer  *http.Server
	runningPort string
	ready       chan struct{}
}

// NewApp 创建并返回一个新的 App 实例
func NewApp(ctx context.Context, config *Config, logger *appLogger) (*App, error) {
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

// Run 启动应用服务
func (a *App) Run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/", a.handleWebSocket)

	port, err := strconv.Atoi(a.config.Port)
	if err != nil {
		return err
	}

	for {
		addr := ":" + strconv.Itoa(port)
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			a.httpServer = newHTTPServer(addr, mux)
			a.runningPort = strconv.Itoa(port)

			close(a.ready)
			a.logger.Info("WebSocket 服务监听中", "port", a.runningPort)
			return a.httpServer.Serve(listener)
		}
		port++
	}
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		IdleTimeout:       httpIdleTimeout,
		MaxHeaderBytes:    httpMaxHeaderBytes,
	}
}

// Shutdown 优雅地关闭应用
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

// handleWebSocket 处理新的 WebSocket 连接请求
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
