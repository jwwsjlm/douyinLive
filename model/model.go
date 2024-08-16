package model

import "DouyinLive/database"

type Manager struct {
	Username string `json:"username"`
	Password string `json:"-"`
	Email    string `json:"email"`
	RoleId   int64  `json:"role_id"`
}

func InsertMessage() {
	manager := Manager{
		Username: "yes",
		Password: "manager password",
		Email:    "this is email",
		RoleId:   1,
	}
	database.DB.Table("managers").Create(&manager)
}
