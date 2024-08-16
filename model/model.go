package model

import "DouyinLive/database"

type Comment struct {
	RoomId  int    `json:"room_id"`
	UserId  int    `json:"user_id"`
	Content string `json:"content"`
}

func InsertComments(roomId, userId int, content string) {
	comment := Comment{
		RoomId:  roomId,
		UserId:  userId,
		Content: content,
	}
	database.DB.Table("comments").Create(&comment)
}
