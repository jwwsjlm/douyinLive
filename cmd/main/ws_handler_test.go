package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/lxzan/gws"
)

type oversizedMessageHandler struct {
	gws.BuiltinEventHandler
	messages atomic.Int32
}

func (h *oversizedMessageHandler) OnMessage(_ *gws.Conn, message *gws.Message) {
	h.messages.Add(1)
	_ = message.Close()
}

func TestDownstreamWebSocketRejectsOversizedClientMessage(t *testing.T) {
	handler := &oversizedMessageHandler{}
	upgrader := gws.NewUpgrader(handler, downstreamServerOption())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := upgrader.Upgrade(w, r)
		if err != nil {
			t.Errorf("Upgrade() failed: %v", err)
			return
		}
		go socket.ReadLoop()
	}))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("Dial() failed: %v", err)
	}
	defer conn.Close()
	if err := conn.WriteMessage(websocket.TextMessage, bytes.Repeat([]byte("x"), downstreamReadMaxPayloadSize+1)); err != nil {
		t.Fatalf("WriteMessage() failed: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("oversized downstream message did not close the connection")
	}
	if got := handler.messages.Load(); got != 0 {
		t.Fatalf("OnMessage called %d times for oversized payload", got)
	}
}

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

func TestWsHandlerNilRoomAndMessageAreIgnored(t *testing.T) {
	handler := NewWsHandler(nil)
	handler.OnOpen(nil)
	handler.OnMessage(nil, nil)
	handler.OnClose(nil, nil)
}

type deadlineLeakHandler struct {
	gws.BuiltinEventHandler
	client *Client
}

type finalCloseMessageHandler struct {
	gws.BuiltinEventHandler
}

func (h *finalCloseMessageHandler) OnOpen(socket *gws.Conn) {
	client := NewClient("close-test", socket)
	go client.close(invalidRoomClientClose)
}

func TestClientCloseSendsCompleteTextMessageThenShortCloseReason(t *testing.T) {
	handler := &finalCloseMessageHandler{}
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

	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("Dial() failed: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() final application message error = %v", err)
	}
	if messageType != websocket.TextMessage || !bytes.Equal(payload, roomInvalidMessage) {
		t.Fatalf("final message type=%d payload=%q", messageType, payload)
	}
	if !json.Valid(payload) {
		t.Fatalf("final application message is not complete JSON: %q", payload)
	}

	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) {
		t.Fatalf("second ReadMessage() error = %v, want CloseError", err)
	}
	if closeErr.Code != websocket.ClosePolicyViolation || closeErr.Text != "room_not_found" {
		t.Fatalf("close code=%d reason=%q", closeErr.Code, closeErr.Text)
	}
}

func (h *deadlineLeakHandler) OnOpen(socket *gws.Conn) {
	h.client = NewClient(socket.RemoteAddr().String(), socket)
	go h.client.writeLoop(nil)
	if !h.client.enqueue(gws.OpcodeText, []byte(`{"type":"system","message":"hello"}`)) {
		panic("failed to enqueue initial message")
	}
}

func (h *deadlineLeakHandler) OnClose(socket *gws.Conn, err error) {
	if h.client != nil {
		h.client.close(normalClientClose)
	}
}

func (h *deadlineLeakHandler) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.WritePong(payload)
}

func TestRoomClientPingStillGetsPongAfterPreviousWriteDeadlineExpires(t *testing.T) {
	handler := &deadlineLeakHandler{}
	upgrader := gws.NewUpgrader(handler, nil)

	var serverReadLoopExited atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := upgrader.Upgrade(w, r)
		if err != nil {
			t.Errorf("Upgrade() failed: %v", err)
			return
		}
		go func() {
			defer serverReadLoopExited.Store(true)
			socket.ReadLoop()
		}()
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Dial() failed: %v", err)
	}
	defer conn.Close()

	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("ReadMessage() for initial room status failed: %v", err)
	}

	time.Sleep(clientWriteTimeout + 500*time.Millisecond)

	gotPong := make(chan string, 1)
	readErrCh := make(chan error, 1)
	conn.SetPongHandler(func(payload string) error {
		gotPong <- payload
		return nil
	})

	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				readErrCh <- err
				return
			}
		}
	}()

	const payload = "keepalive-after-server-write"
	if err := conn.WriteControl(websocket.PingMessage, []byte(payload), time.Now().Add(time.Second)); err != nil {
		t.Fatalf("WriteControl(PingMessage) failed: %v", err)
	}

	select {
	case got := <-gotPong:
		if got != payload {
			t.Fatalf("pong payload = %q, want %q", got, payload)
		}
	case err := <-readErrCh:
		t.Fatalf("connection closed before pong: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for pong")
	}

	if serverReadLoopExited.Load() {
		t.Fatalf("server read loop exited unexpectedly after ping/pong")
	}
}
