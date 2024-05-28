### wss链接抖音抓取弹幕礼物数据

---



项目核心来自 https://github.com/saermart/DouyinLiveWebFetcher 

改写成golang测试

新增ws链接
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
链接地址为`ws://127.0.0.1:18080/ws` 
内置了个简单的心跳包.客户端需要每隔30s发送一次`ping`
![1716881601495.png](image%2FREADME%2F1716881601495.png)