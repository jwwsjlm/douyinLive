package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lxzan/gws"
)

type pingTestHandler struct {
	*WsHandler
}

func (h *pingTestHandler) OnOpen(socket *gws.Conn) {}

func (h *pingTestHandler) OnClose(socket *gws.Conn, err error) {}

func (h *pingTestHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	_ = message.Close()
}

func TestWsHandlerRepliesPongWithPingPayload(t *testing.T) {
	handler := &pingTestHandler{WsHandler: &WsHandler{}}
	upgrader := gws.NewUpgrader(handler, nil)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := upgrader.Upgrade(w, r)
		if err != nil {
			t.Errorf("Upgrade() failed: %v", err)
			return
		}
		go socket.ReadLoop()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() failed: %v", err)
	}
	defer conn.Close()

	gotPong := make(chan string, 1)
	conn.SetPongHandler(func(payload string) error {
		gotPong <- payload
		return nil
	})

	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	const payload = "client-heartbeat"
	deadline := time.Now().Add(time.Second)
	if err := conn.WriteControl(websocket.PingMessage, []byte(payload), deadline); err != nil {
		t.Fatalf("WriteControl(PingMessage) failed: %v", err)
	}

	select {
	case got := <-gotPong:
		if got != payload {
			t.Fatalf("pong payload = %q, want %q", got, payload)
		}
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for pong")
	}
}
