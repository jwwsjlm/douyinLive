package douyinLive

import (
	"bytes"
	"compress/gzip"
	"strings"
	"sync"
	"testing"

	"github.com/jwwsjlm/douyinLive/v2/generated/new_douyin"
	"google.golang.org/protobuf/proto"
)

func TestQueryEscapeValuePreservesSignatureCharacters(t *testing.T) {
	got := queryEscapeValue("ab+c/d e")
	want := "ab%2Bc%2Fd%20e"
	if got != want {
		t.Fatalf("queryEscapeValue() = %q, want %q", got, want)
	}
}

func TestParseRoomInfoSupportsRoomObject(t *testing.T) {
	body := `{
		"data": {
			"room": {
				"id": "room-id",
				"title": "room-title",
				"owner": {
					"id": "owner-id",
					"nickname": "owner-name",
					"avatar_thumb": {
						"url_list": ["avatar-0", "avatar-1", "avatar-2"]
					}
				}
			}
		}
	}`

	info, err := parseRoomInfo(body)
	if err != nil {
		t.Fatalf("parseRoomInfo() failed: %v", err)
	}
	if info.roomID != "room-id" || info.pushID != "owner-id" {
		t.Fatalf("unexpected ids: roomID=%q pushID=%q", info.roomID, info.pushID)
	}
	if info.liveName != "owner-name" || info.title != "room-title" || info.avatarThumb != "avatar-2" {
		t.Fatalf("unexpected room info: %#v", info)
	}
}

func TestDecodeGzipResponseAcceptsNormalPayload(t *testing.T) {
	payload, err := proto.Marshal(&new_douyin.Webcast_Im_Response{})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	compressed := gzipTestPayload(t, payload)
	dl := newTestDouyinLiveWithBufferPool()

	err = dl.decodeGzipResponse(
		compressed,
		&new_douyin.Webcast_Im_PushFrame{},
		&new_douyin.Webcast_Im_Response{},
		&new_douyin.Webcast_Im_ControlMessage{},
	)
	if err != nil {
		t.Fatalf("decodeGzipResponse() failed: %v", err)
	}
}

func TestDecodeGzipResponseRejectsOversizedPayload(t *testing.T) {
	if maxGzipPayloadSize < 32<<20 {
		t.Fatalf("maxGzipPayloadSize = %d, want at least 32MiB", maxGzipPayloadSize)
	}
	compressed := gzipTestPayload(t, []byte(strings.Repeat("x", maxGzipPayloadSize+1)))
	dl := newTestDouyinLiveWithBufferPool()

	err := dl.decodeGzipResponse(
		compressed,
		&new_douyin.Webcast_Im_PushFrame{},
		&new_douyin.Webcast_Im_Response{},
		&new_douyin.Webcast_Im_ControlMessage{},
	)
	if err == nil {
		t.Fatalf("decodeGzipResponse() returned nil error for oversized payload")
	}
}

func newTestDouyinLiveWithBufferPool() *DouyinLive {
	return &DouyinLive{
		bufferPool: &sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, gzipBufferSize))
			},
		},
	}
}

func gzipTestPayload(t *testing.T, payload []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(payload); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}
