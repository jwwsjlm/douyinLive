package main

import (
	"fmt"
	"github.com/jwwsjlm/douyinlive/sign"
)

func main() {
	params := "aid=6383&app_name=douyin_web&live_id=1&device_platform=web&language=zh-CN&web_rid=123456"
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36"
	
	aBogus := sign.AbSign(params, ua)
	fmt.Println("✅ 签名生成成功！")
	fmt.Println("a_bogus:", aBogus)
	fmt.Println("\n完整 URL:")
	fmt.Printf("https://live.douyin.com/webcast/room/web/enter/?%s&a_bogus=%s\n", params, aBogus)
}
