# `sign` 包说明

[返回项目首页](../README.md)

`sign` 是项目中的公开 Go 子包，负责两类基础能力：

- 生成 HTTP 请求使用的 `a_bogus` 参数；
- 管理 Cookie 配置、Cookie 解析和 `http.CookieJar`。

应用主程序会通过上层会话配置自动使用这些能力。普通用户不需要直接调用本包；只有将 `douyinLive` 作为 Go 库集成，或需要单独测试签名/Cookie 逻辑时，才建议直接使用它。

## 安装

```bash
go get github.com/jwwsjlm/douyinLive/v2/sign
```

## `a_bogus` 签名

`AbSign` 根据 URL 查询参数和 User-Agent 生成 `a_bogus`：

```go
package main

import (
    "fmt"

    "github.com/jwwsjlm/douyinLive/v2/sign"
)

func main() {
    params := "aid=6383&app_name=douyin_web&room_id=123456789"
    userAgent := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"

    aBogus := sign.AbSign(params, userAgent)
    fmt.Println(aBogus)
}
```

注意：

- `params` 应该是实际请求使用的 URL 查询参数字符串，不要把已经拼接的 `a_bogus` 再传入；
- User-Agent 必须与发起 HTTP 请求时使用的 User-Agent 保持一致；
- 签名包含时间相关数据，同样的输入重复调用不保证得到相同字符串；
- `AbSign` 是同步函数，包内测试覆盖了并发调用场景；
- 它只生成签名，不负责发起 HTTP 请求，也不负责 Cookie、重试或 WebSocket 连接。

当前实现使用内置 SM3、RC4 和变体 Base64 逻辑，不依赖 JavaScript Runtime。WebSocket `signature` 不由本包生成，而由上层 WebSocket signer 负责；不要将两种签名混用。

## CookieManager

`CookieManager` 提供 Cookie 的读取、设置、解析、校验和 Cookie Jar 管理：

```go
package main

import (
    "fmt"

    "github.com/jwwsjlm/douyinLive/v2/sign"
)

func main() {
    manager := sign.NewCookieManager()
    manager.SetDouyinCookie("ttwid=example; sessionid=example")

    fmt.Println(manager.GetDouyinCookie())
    fmt.Println(manager.GetCookieNames(manager.GetDouyinCookie()))

    if err := manager.SetCookies(
        "https://live.douyin.com/",
        manager.GetDouyinCookie(),
    ); err != nil {
        panic(err)
    }

    cookies := manager.GetCookies("https://live.douyin.com/room")
    fmt.Println(len(cookies))
}
```

常用方法：

| 方法 | 作用 |
| --- | --- |
| `NewCookieManager()` | 创建 Cookie 管理器 |
| `LoadConfig(path)` | 从 YAML 文件读取 `cookie.douyin` |
| `LoadFromEnv()` | 读取 `DOUYIN_COOKIE`，仅适用于直接使用 `CookieManager` |
| `GetDouyinCookie()` | 获取当前默认 Cookie 字符串 |
| `SetDouyinCookie(cookie)` | 设置默认 Cookie 字符串 |
| `ParseCookies(cookie)` | 解析 Cookie 请求头字符串 |
| `SetCookies(rawURL, cookie)` | 将 Cookie 写入对应 URL 的 Jar |
| `GetCookies(rawURL)` | 从 Jar 获取对应 URL 的 Cookie |
| `UpdateCookie(name, value)` | 更新配置中的 `douyin` Cookie |
| `SaveConfig(path)` | 将当前 Cookie 配置保存为 YAML |
| `ValidateCookie(cookie)` | 检查是否包含常见抖音 Cookie 字段 |
| `GetCookieNames(cookie)` | 获取 Cookie 名称列表 |

### Cookie YAML 格式

```yaml
cookie:
  douyin: "ttwid=...; sessionid=..."
```

`CookieManager.LoadConfig` 只读取上述 `cookie.douyin` 字段。应用主程序的完整配置（包括按房间 Cookie、日志、签名来源和轮询间隔）请参考 [`docs/configuration.md`](../docs/configuration.md)，不要把 `CookieManager` 的简化 YAML 格式和主程序配置混为一谈。

### 环境变量的区别

直接调用 `CookieManager.LoadFromEnv()` 时读取的是：

```text
DOUYIN_COOKIE
```

独立服务程序使用 Viper 配置体系，环境变量按 `APP_` 前缀映射，例如：

```text
APP_COOKIE_DOUYIN
APP_SIGN_PROVIDER
APP_TIKHUB_KEY
```

命令行和配置文件的完整优先级请参考 [`docs/configuration.md`](../docs/configuration.md)。

## 安全和生命周期

- Cookie、Token、TikHub Key 不要提交到 Git，也不要写入公开 Issue 或完整日志；
- `CookieManager` 不负责自动刷新 Cookie，Cookie 失效后需要调用方更新；
- `GetCookies` 对无效 URL 返回 `nil`，`SetCookies` 对无效 URL 返回错误；
- 不要在多个并发会话之间共享同一个 `CookieManager`，建议每个直播会话独立创建；
- `CookieManager` 没有 `Close` 方法，停止使用后释放调用方对它的引用即可；
- 主程序和 `DouyinLive` 库会在会话关闭时释放 HTTP 空闲连接及签名 Runtime，库模式请按文档调用 `Close`/`Dispose`。

## 测试

在仓库根目录运行：

```bash
go test ./sign
go test -race ./sign
go test ./...
```

`sign/ab_sign_test.go` 包含 SM3 官方向量、签名格式、并发调用和 Benchmark；`sign/cookie_manager_test.go` 覆盖配置读写、环境变量、Cookie 解析及 Jar 往返。

## 相关文档

- [`docs/configuration.md`](../docs/configuration.md)：独立服务配置、Cookie 优先级和环境变量；
- [`docs/library.md`](../docs/library.md)：作为 Go 库使用、订阅和生命周期；
- [`docs/cli.md`](../docs/cli.md)：命令行参数、日志和签名来源切换；
- [`docs/websocket-client.md`](../docs/websocket-client.md)：本地 WebSocket 客户端接入。
