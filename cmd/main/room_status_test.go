package main

import (
	"encoding/json"
	"testing"
)

func TestBuildEventJSONHandlesEmptyObject(t *testing.T) {
	room := &Room{}
	got, err := room.buildEventJSON([]byte(`{}`), "WebcastRoomMessage", "主播", "标题", "https://example.test/avatar")
	if err != nil {
		t.Fatalf("buildEventJSON() error = %v", err)
	}
	if !json.Valid(got) {
		t.Fatalf("buildEventJSON() returned invalid JSON: %s", got)
	}
	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["method"] != "WebcastRoomMessage" || payload["livename"] != "主播" || payload["title"] != "标题" {
		t.Fatalf("unexpected enriched payload: %#v", payload)
	}
}

func TestBuildEventJSONPreservesExistingObject(t *testing.T) {
	room := &Room{}
	got, err := room.buildEventJSON([]byte(" \n {\"count\":1} \n"), "WebcastRoomStatsMessage", "", "", "")
	if err != nil {
		t.Fatalf("buildEventJSON() error = %v", err)
	}
	if !json.Valid(got) {
		t.Fatalf("buildEventJSON() returned invalid JSON: %s", got)
	}
	var payload map[string]any
	if err := json.Unmarshal(got, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if payload["count"] != float64(1) || payload["method"] != "WebcastRoomStatsMessage" {
		t.Fatalf("unexpected enriched payload: %#v", payload)
	}
}

func TestBuildEventJSONRejectsInvalidOrNonObjectJSON(t *testing.T) {
	room := &Room{}
	for _, raw := range [][]byte{nil, []byte(`{`), []byte(`[]`), []byte(`{"broken":}`)} {
		if _, err := room.buildEventJSON(raw, "method", "", "", ""); err == nil {
			t.Fatalf("buildEventJSON(%q) unexpectedly succeeded", raw)
		}
	}
}
