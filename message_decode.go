package douyinLive

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"

	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
	"google.golang.org/protobuf/proto"
)

// handleGzipMessage 处理 GZIP 消息
// handleGzipMessage 解压并解析 gzip 编码的 PushFrame 载荷。
// handleGzipMessage decompresses and decodes a gzip-encoded PushFrame payload.
func (dl *DouyinLive) handleGzipMessage(pushFrame *new_douyin.Webcast_Im_PushFrame, response *new_douyin.Webcast_Im_Response, controlMsg *new_douyin.Webcast_Im_ControlMessage) {
	if err := dl.decodeGzipResponse(pushFrame.Payload, pushFrame, response, controlMsg); err != nil {
		dl.logger.Warn("解析 GZIP Response 失败", "live_id", dl.liveID, "payload_len", len(pushFrame.Payload), "err", err)
	}
}

// handlePlainMessage 解析未压缩的 PushFrame 载荷。
// handlePlainMessage decodes an uncompressed PushFrame payload.
func (dl *DouyinLive) handlePlainMessage(pushFrame *new_douyin.Webcast_Im_PushFrame, response *new_douyin.Webcast_Im_Response, controlMsg *new_douyin.Webcast_Im_ControlMessage) {
	if err := dl.decodeResponse(pushFrame.Payload, pushFrame, response, controlMsg); err != nil {
		dl.logger.Warn("解析 Response 失败", "live_id", dl.liveID, "payload_len", len(pushFrame.Payload), "err", err)
	}
}

// decodeResponse 反序列化响应、按需 ACK，并分发其中的业务消息。
// decodeResponse unmarshals a response, sends ACK when needed, and dispatches contained messages.
func (dl *DouyinLive) decodeResponse(data []byte, pushFrame *new_douyin.Webcast_Im_PushFrame, response *new_douyin.Webcast_Im_Response, controlMsg *new_douyin.Webcast_Im_ControlMessage) error {
	*response = new_douyin.Webcast_Im_Response{}
	if err := proto.Unmarshal(data, response); err != nil {
		return err
	}

	dl.applyWebsocketResponseState(response)

	if response.NeedAck {
		dl.sendAck(pushFrame.LogID, response.InternalExt)
	}

	for _, msg := range response.Messages {
		if dl.isManualClose() || !dl.isLiveStatus() {
			break
		}
		dl.handleSingleMessage(msg, controlMsg)
	}
	return nil
}

// decodeGzipResponse 解压 gzip 响应并复用普通响应解析流程。
// decodeGzipResponse decompresses a gzip response and reuses the normal response decoder.
func (dl *DouyinLive) decodeGzipResponse(data []byte, pushFrame *new_douyin.Webcast_Im_PushFrame, response *new_douyin.Webcast_Im_Response, controlMsg *new_douyin.Webcast_Im_ControlMessage) error {
	buf := dl.bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		if buf.Cap() > maxGzipPayloadSize {
			return
		}
		buf.Reset()
		dl.bufferPool.Put(buf)
	}()

	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()

	if _, err = buf.ReadFrom(io.LimitReader(gz, maxGzipPayloadSize+1)); err != nil {
		return err
	}
	if buf.Len() > maxGzipPayloadSize {
		return fmt.Errorf("gzip payload too large: %d bytes", buf.Len())
	}

	return dl.decodeResponse(buf.Bytes(), pushFrame, response, controlMsg)
}

// handleSingleMessage 处理单条消息
// handleSingleMessage 处理单条业务消息，并识别下播控制消息。
// handleSingleMessage handles one business message and detects live-end control messages.
func (dl *DouyinLive) handleSingleMessage(msg *new_douyin.Webcast_Im_Message,
	controlMsg *new_douyin.Webcast_Im_ControlMessage) {
	if dl.isManualClose() || !dl.isLiveStatus() {
		return
	}

	if msg.Method == "WebcastControlMessage" {
		if err := proto.Unmarshal(msg.Payload, controlMsg); err != nil {
			dl.logger.Warn("解析控制消息失败", "live_id", dl.liveID, "payload_len", len(msg.Payload), "err", err)
			return
		}
		dl.emitEvent(msg, controlMsg)
		if controlMsg.GetAction() == controlActionLiveEnd {
			dl.logger.Info("收到直播结束控制消息", "live_id", dl.liveID, "live_name", dl.GetName(), "action", controlMsg.GetAction())
			dl.setLiveStatus(false)
		}
		return
	}

	dl.emitEvent(msg, nil)
}
