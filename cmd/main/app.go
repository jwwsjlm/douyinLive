package main

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lxzan/gws"
)

// App 是应用的核心结构体，封装了所有依赖
type App struct {
	ctx         context.Context
	logger      *log.Logger
	config      *Config
	roomManager *RoomManager
	httpServer  *http.Server
	runningPort string
}

// NewApp 创建并返回一个新的 App 实例
func NewApp(ctx context.Context, config *Config, logger *log.Logger) (*App, error) {
	roomManager := NewRoomManager(logger, config.Unknown)
	return &App{
		ctx:         ctx,
		logger:      logger,
		config:      config,
		roomManager: roomManager,
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

	// 查找可用端口
	for {
		addr := ":" + strconv.Itoa(port)
		if isPortAvailable(port) {
			a.httpServer = &http.Server{Addr: addr, Handler: mux}
			a.runningPort = strconv.Itoa(port)
			return a.httpServer.ListenAndServe()
		}
		port++
	}
}

// Shutdown 优雅地关闭应用
func (a *App) Shutdown() error {
	a.logger.Println("正在关闭 RoomManager...")
	a.roomManager.CloseAll()

	if a.httpServer != nil {
		a.logger.Println("正在关闭 HTTP 服务...")
		shutdownCtx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
		defer cancel()
		return a.httpServer.Shutdown(shutdownCtx)
	}
	return nil
}

// handleWebSocket 处理新的 WebSocket 连接请求
func (a *App) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	roomID := strings.TrimPrefix(r.URL.Path, "/ws/")
	if roomID == "" {
		http.Error(w, "无效的房间ID", http.StatusBadRequest)
		return
	}

	a.logger.Printf("接收到 WebSocket 连接请求, 房间ID: %s, 客户端: %s", roomID, r.RemoteAddr)

	room := a.roomManager.GetOrCreateRoom(roomID)
	handler := NewWsHandler(room)

	upgrader := gws.NewUpgrader(handler, &gws.ServerOption{
		ParallelEnabled:   true,
		Recovery:          gws.Recovery,
		PermessageDeflate: gws.PermessageDeflate{Enabled: true},
	})

	socket, err := upgrader.Upgrade(w, r)
	if err != nil {
		a.logger.Printf("升级 WebSocket 失败: %v", err)
		return
	}

	go socket.ReadLoop()
}
