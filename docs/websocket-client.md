# WebSocket 客户端与消息格式

[返回项目首页](../README.md)

本文介绍客户端连接方式、状态消息、业务消息和推荐处理逻辑。

如果你直接运行独立服务，你的客户端只需要连本地 WebSocket 服务即可。

## JavaScript 示例

```javascript
const ws = new WebSocket('ws://127.0.0.1:1088/ws/516466932480');

ws.onopen = () => {
  console.log('已连接');
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('收到消息:', data);

  if (data.event === 'live_status') {
    if (data.live) {
      console.log('状态通知: 直播间已开播');
    } else if (data.ended) {
      console.log(`状态通知: ${data.message}，后续会继续按 ${data.retry_interval_seconds} 秒轮询`);
    } else {
      console.log(`状态通知: ${data.message}，${data.retry_interval_seconds} 秒后重试`);
    }
    return;
  }

  switch (data.method) {
    case 'WebcastChatMessage':
      console.log(`弹幕: ${data.user.nickname} - ${data.content}`);
      break;
    case 'WebcastGiftMessage':
      console.log(`礼物: ${data.user.nickname} 赠送了 ${data.gift.name}`);
      break;
    case 'WebcastLikeMessage':
      console.log(`${data.user.nickname} 点赞了直播间`);
      break;
    default:
      break;
  }
};

ws.onclose = () => {
  console.log('连接关闭');
};

ws.onerror = (err) => {
  console.error('WebSocket 错误:', err);
};

// 可选：给本地服务发文本 ping，服务会回文本 pong
setInterval(() => {
  if (ws.readyState === WebSocket.OPEN) {
    ws.send('ping');
  }
}, 30000);
```

浏览器端不能直接发送 WebSocket ping 控制帧，所以示例里使用文本 `"ping"`。如果你的客户端库支持 WebSocket ping frame，也可以发送标准 ping 控制帧，服务端会按规范用相同 payload 回复 pong。

## 服务端返回什么格式

服务端会返回两类 JSON 文本：

1. **系统状态消息**：`type="system"`，用于告诉客户端直播间当前处于什么状态。
2. **直播业务消息**：抖音 WebSocket 下发的 protobuf 消息转成 JSON 后再推给客户端。

### 系统状态消息字段说明

| 字段 | 含义 |
| --- | --- |
| `type` | 固定为 `system`，表示这是服务端状态通知，不是弹幕或礼物消息。 |
| `event` | 固定为 `live_status` 时，表示直播间状态变化。 |
| `code` | 给程序判断用的稳定状态码，例如 `ROOM_OFFLINE`、`ROOM_ONLINE`。 |
| `status` | 简短英文状态：`offline`、`online`、`ended`、`not_found`、`error`。 |
| `status_text` | 给用户看的中文状态，例如“直播间未开播”。 |
| `valid` | `true` 表示账号或直播间有效；`false` 表示无效 ID 或状态检查失败。 |
| `live` | `true` 表示已开播；`false` 表示未开播、已下播或异常。 |
| `message` | 给用户直接展示的中文说明。 |
| `suggestion` | 给客户端或用户的下一步建议。 |
| `room_id` | 用户连接时传入的直播间标识。 |
| `live_name` | 主播昵称；如果网页没有返回，可能为空字符串。 |
| `title` | 直播间标题；账号存在但当前没有直播间对象时可能为空字符串。 |
| `avatar_thumb` | 主播头像缩略图地址；没有取到时为空字符串。 |
| `has_room` | `true` 表示已确认存在直播间对象，`false` 表示仅确认账号存在但没有房间，`null` 表示当前响应不足以判断。 |
| `account_only` | `true` 表示仅存在账号，`false` 表示已确认存在房间，`null` 表示当前响应不足以判断。 |
| `retry_interval_seconds` | 未开播/已下播时，服务端下一次轮询的大致间隔。 |

### 情况一：直播间未开播，但账号/直播间有效

服务端会保持本地 WebSocket 连接，并持续后台轮询，不会关闭客户端。

```json
{
  "type": "system",
  "event": "live_status",
  "code": "ROOM_OFFLINE",
  "valid": true,
  "live": false,
  "status": "offline",
  "status_text": "直播间未开播",
  "room_id": "32536162943",
  "live_name": "一只喵动漫",
  "title": "",
  "avatar_thumb": "https://example.com/avatar.jpeg",
  "message": "直播间当前未开播，服务端会保持连接并继续轮询",
  "suggestion": "客户端不需要重连，保持当前 WebSocket 连接等待开播通知",
  "retry_interval_seconds": 30
}
```

客户端处理建议：不要断开，不要立即重连，显示“未开播，等待中”即可。

### 情况二：账号存在，但当前没有直播间房间对象

有些短号或主页号能打开账号页，但网页没有返回 `roomInfo.room`，只返回了 `roomInfo.anchor`。这说明账号存在，只是当前没有直播间房间对象，常见于账号从未开播过、或当前没有创建直播间房间对象。服务端会把它当成“未开播等待中”，并保持本地 WebSocket 连接。判断依据以网页 SSR 状态为准：只要没有 `roomInfo.room`，即使旁边存在 `roomId` 字段，也不会当作已存在的直播间房间对象。

```json
{
  "type": "system",
  "event": "live_status",
  "code": "ACCOUNT_OFFLINE_NO_ROOM",
  "valid": true,
  "live": false,
  "status": "account_offline",
  "status_text": "账号存在但当前没有直播间",
  "room_id": "32536162943",
  "live_name": "一只喵动漫",
  "title": "",
  "avatar_thumb": "https://example.com/avatar.jpeg",
  "has_room": false,
  "account_only": true,
  "message": "账号存在，但网页没有返回直播间房间对象，可能是该账号从未开播或当前未创建直播间，当前按未开播处理",
  "suggestion": "客户端不需要重连，保持当前 WebSocket 连接；如果该账号后续开播，服务端会自动切换为直播连接",
  "retry_interval_seconds": 30
}
```

客户端处理建议：和未开播一样处理，不要断开，不要立即重连。

### 情况三：直播间已开播

```json
{
  "type": "system",
  "event": "live_status",
  "code": "ROOM_ONLINE",
  "valid": true,
  "live": true,
  "status": "online",
  "status_text": "直播间已开播",
  "room_id": "536681248455",
  "live_name": "主播昵称",
  "title": "直播间标题",
  "avatar_thumb": "https://example.com/avatar.jpeg",
  "message": "直播间已开播，后续将开始推送弹幕、礼物、点赞等直播消息",
  "suggestion": "客户端可以开始正常处理直播消息"
}
```

客户端处理建议：进入正常消息处理流程。

`ROOM_ONLINE` 只会在上游抖音 WebSocket 完成握手并进入消息接收流程后发送。客户端连接本地服务后，如果暂时没有立即收到状态消息，应保持连接，不要通过关闭/重新打开弹幕开关来触发重连。

### 情况四：直播过程中下播

服务端会推送已下播状态，然后切回后台轮询，等待再次开播。

```json
{
  "type": "system",
  "event": "live_status",
  "code": "ROOM_ENDED",
  "valid": true,
  "live": false,
  "status": "ended",
  "status_text": "直播间已下播",
  "room_id": "386395296025",
  "live_name": "CACA呆夫（无畏契约）",
  "title": "奶妈王来了",
  "avatar_thumb": "https://example.com/avatar.jpeg",
  "message": "直播间已经下播，服务端会保持连接并等待再次开播",
  "suggestion": "客户端不需要重连，保持当前 WebSocket 连接等待下一次开播",
  "ended": true,
  "retry_interval_seconds": 30
}
```

### 情况五：直播间不存在或 ID 无效

这种情况服务端会关闭当前客户端连接，因为继续轮询没有意义。

```json
{
  "type": "system",
  "event": "live_status",
  "code": "ROOM_NOT_FOUND",
  "valid": false,
  "live": false,
  "status": "not_found",
  "status_text": "直播间不存在或房间号无效",
  "message": "直播间不存在或房间号无效，已关闭连接",
  "suggestion": "请检查直播间ID是否输入正确；如果是短号或主页号，请确认网页可以正常打开该账号或直播间"
}
```

### 情况六：状态暂时无法确认

这种情况通常是匿名访问遇到验证码、网络波动、Cookie 失效、风控或网页结构变化导致。服务端不会把它误判为未开播或房间不存在，也不会立即关闭客户端；客户端应保持当前连接，由服务端继续轮询。无 Cookie 模式会自动获取 `ttwid`，但不能保证绕过抖音对匿名高频访问触发的验证码。

```json
{
  "type": "system",
  "event": "live_status",
  "code": "ROOM_STATUS_UNKNOWN",
  "valid": false,
  "live": null,
  "has_room": null,
  "account_only": null,
  "status": "unknown",
  "status_text": "直播状态暂时无法确认",
  "message": "暂时无法确认直播状态，服务端会保持连接并继续轮询",
  "suggestion": "请保持当前连接；如果长时间不能恢复，请开启 debug 日志并检查 Cookie 或验证码风控"
}
```

客户端不要在收到 `ROOM_STATUS_UNKNOWN` 后立即反复重连。频繁创建匿名会话更容易触发验证码；同一连接会复用房间级探测画像，连续失败达到阈值后才自动轮换 UA、浏览器指纹、`ttwid` 和 HTTP Client。

### 直播业务消息

直播间正常开播后，服务端会把解析后的 protobuf 消息转成 JSON 文本发给你。不同消息类型字段不完全一样，但都会尽量补充以下元信息：

- `method`：抖音消息类型，例如 `WebcastChatMessage`、`WebcastGiftMessage`。
- `livename`：主播昵称。
- `title`：直播间标题。
- `avatarThumb`：主播头像缩略图地址。

示例：

```json
{
  "method": "WebcastChatMessage",
  "livename": "主播昵称",
  "title": "直播间标题",
  "avatarThumb": "https://example.com/avatar.jpeg"
}
```


---
