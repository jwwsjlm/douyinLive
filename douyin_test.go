package douyinLive

import "testing"

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
