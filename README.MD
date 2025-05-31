### wss链接抖音抓取弹幕礼物数据

---



项目灵感来自:https://github.com/saermart/DouyinLiveWebFetcher 

项目proto来自:https://github.com/Remember-the-past/douyin_proto

感谢上面两位的无私贡献,

目前改动

```
正常运行之后
ws://127.0.0.1:1088/ws/直播间
即可实现单进程监控多直播间,挂一个到服务器就好了,客户端直接比如要链接直播间A ws://127.0.0.1:1088/ws/AAAAA
需要监控B的话同理 只开一个进程即可
```

测试在main/main.go中

    go run main/main.go --room ****抖音直播间号**** --port ****端口号****(默认为18080) --unknown false(默认为false)
    unknown为是否输出未知数据.因为现在源pb还不是很全.有部分消息无法解析
    最终拼接为
    go run main/main.go --room 933572413882 --port 18080
    也可以go build main/main.go编译成二进制文件运行
    最终运行命令为
    `main.exe --room 933572413882`

![20240528200625.png](image%2FREADME%2F20240528200625.png)
软件通过json格式输出数据.请自行解析
链接地址为`ws://127.0.0.1:1088/ws`
内置了个简单的心跳包.客户端需要每隔30s发送一次`ping`
![1716881601495.png](image%2FREADME%2F1716881601495.png)
有能力的可以完善下proto文件 抖音的proto相关的链接在
https://lf-cdn-tos.bytescm.com/obj/static/webcast/douyin_live/chunks/live-schema.0fa7e4bc.js
或者全局搜索`webcast.im.Common`也可定位相关函数