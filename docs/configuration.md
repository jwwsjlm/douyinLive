# 配置文件

[返回项目首页](../README.md)

本文说明 `config.yaml`、环境变量、Cookie、签名方式和配置优先级。

你可以创建一个 `config.yaml` 放在程序同目录下。

示例：

```yaml
port: "1088"
unknown: false
log:
  level: "info"
sign:
  provider: ""
tikhub:
  key: ""
monitor:
  poll_interval: "15s"
  notify_interval: "30s"
cookie:
  douyin: ""
  rooms:
    # "516466932480": "ttwid=...; sessionid=..."
```

项目里也自带了一个示例文件：

- `config.example.yaml`

## 配置项说明

### `port`
本地 WebSocket 服务端口。

默认值：

```yaml
port: "1088"
```

### `unknown`
是否打印未知消息类型。

默认值：

```yaml
unknown: false
```

### `log.level`

日志级别。默认输出 `info` 及以上级别，排查连接、心跳、重连问题时可以临时调整为 `debug`。

默认值：

```yaml
log:
  level: "info"
```

### `sign.provider`

WebSocket 签名来源。可选值：

- `local`：使用内置原生 Go 签名，WebSocket 签名失败时自动使用独立 Goja Runtime 兼容回退，默认推荐。
- `tikhub`：使用 TikHub 在线 API 生成签名，需要配置 `tikhub.key`。

默认值：

```yaml
sign:
  provider: ""
```

留空表示使用当前二进制默认值，也就是 `local`。如果你想强制指定，也可以写成 `local` 或 `tikhub`。

### `tikhub.key`

TikHub API Key，仅当 `sign.provider` 为 `tikhub` 时需要。

如果选择了 `tikhub` 但没有提供 Key，程序会在启动阶段直接报错，不会等到连接直播间后才失败。日志不会输出完整 API Key。

获取方式：

1. 打开 [TikHub 注册页](https://user.tikhub.io/register) 注册账号
2. 登录 [TikHub 用户中心](https://user.tikhub.io/)
3. 创建 API Key / API Token
4. 把 Key 保存到本地 `config.yaml`

配置写法：

```yaml
sign:
  provider: "tikhub"
tikhub:
  key: "YOUR_TIKHUB_KEY"
```

也可以通过环境变量传入，适合 Docker、systemd、CI 等不想把 Key 写进配置文件的场景：

```bash
APP_SIGN_PROVIDER=tikhub APP_TIKHUB_KEY=YOUR_TIKHUB_KEY ./douyinLive
```

### `monitor.poll_interval`
未开播时，服务端检查“是否已经开播”的时间间隔。

默认值：

```yaml
monitor:
  poll_interval: "15s"
```

### `monitor.notify_interval`
未开播时，服务端向本地 WebSocket 客户端重复推送状态通知的时间间隔。

默认值：

```yaml
monitor:
  notify_interval: "30s"
```

客户端会收到类似：

```json
{"type":"system","event":"live_status","live":false,"room_id":"516466932480","message":"直播间未开播","retry_interval_seconds":30}
```

### `cookie.douyin`
抖音默认 Cookie，可选。

没有单独配置某个直播间的 Cookie 时，会优先回退到这里。再往后才是自动获取的逻辑。

```yaml
cookie:
  douyin: "ttwid=...; sessionid=..."
```

### `cookie.rooms`
按直播间 ID 单独配置 Cookie，可选。

如果你要同时监听多个直播间，而且它们对应不同账号、不同登录态，就可以在这里分别配置。没有配置到的直播间，会自动回退使用 `cookie.douyin`。

```yaml
cookie:
  douyin: "默认 Cookie"
  rooms:
    "516466932480": "直播间 516466932480 专用 Cookie"
    "123456789": "直播间 123456789 专用 Cookie"
    "888888888": "直播间 888888888 专用 Cookie"
```

一个更完整的例子：

```yaml
port: "1088"
unknown: false
log:
  level: "info"
sign:
  provider: ""
tikhub:
  key: ""
monitor:
  poll_interval: "15s"
  notify_interval: "30s"
cookie:
  douyin: "默认 Cookie"
  rooms:
    "516466932480": "room A 的 Cookie"
    "123456789": "room B 的 Cookie"
```

Cookie 优先级：

```text
WebSocket 临时 Cookie > 直播间 Cookie(cookie.rooms) > 默认 Cookie(cookie.douyin) > 自动获取
```

WebSocket 临时 Cookie 仅建议临时调试使用：

```text
ws://127.0.0.1:1088/ws/直播间ID?cookie_b64=BASE64URL_COOKIE
```

也支持直接传 URL 编码后的 Cookie：

```text
ws://127.0.0.1:1088/ws/直播间ID?cookie=URL_ENCODED_COOKIE
```

## 什么时候需要 Cookie

不是所有场景都必须填 Cookie。

你可以先不填，直接跑。

如果出现下面这些情况，再考虑补 Cookie：

- 某些直播间拿不到消息
- 请求被限制
- 页面返回结果异常
- 需要更稳定的登录态

## Cookie 怎么拿

1. 浏览器打开：`https://live.douyin.com`
2. 登录抖音
3. 按 `F12`
4. 打开 `Network`
5. 随便点一个请求
6. 复制请求头里的 `Cookie`

然后填到：

```yaml
cookie:
  douyin: "你的完整 Cookie"
```

---
