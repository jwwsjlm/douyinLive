# Cookie 配置和使用说明

## 📚 参考项目

本 Cookie 处理方案参考了以下开源项目：

### [DouyinLiveRecorder](https://github.com/ihmily/DouyinLiveRecorder)

- **作者**: Hmily
- **GitHub**: https://github.com/ihmily/DouyinLiveRecorder
- **参考内容**:
    - Cookie 配置文件格式（config.ini）
    - Cookie 读取和传入方式
    - 多平台 Cookie 管理方案

---

## 📋 DouyinLiveRecorder 的 Cookie 处理方式

### 1. **配置文件格式** (config/config.ini)

DouyinLiveRecorder 使用 INI 格式存储各平台 Cookie：

```ini
[Cookie]
# 录制抖音必填
抖音cookie = ttwid=1%7CB1qls3GdnZhUov9o2NxOMxxYS2ff6OSvEWbv0ytbES4%7C1680522049%7C280d802d6d478e3e78d0c807f7c487e7ffec0ae4e5fdd6a0fe74c3c6af149511; my_rd=1; passport_csrf_token=3ab34460fa656183fccfb904b16ff742; ...
快手cookie =
tiktok_cookie =
虎牙cookie =
斗鱼cookie =
# ... 其他平台
```

**代码位置**: [`config/config.ini`](https://github.com/ihmily/DouyinLiveRecorder/blob/main/config/config.ini)

### 2. **Cookie 读取方式**

在 [`main.py`](https://github.com/ihmily/DouyinLiveRecorder/blob/main/main.py#L1876-L1900) 中：

```python
# 从配置文件读取 Cookie
dy_cookie = read_config_value(config, 'Cookie', '抖音cookie', '')
ks_cookie = read_config_value(config, 'Cookie', '快手cookie', '')
tiktok_cookie = read_config_value(config, 'Cookie', 'tiktok_cookie', '')
# ... 其他平台
```

### 3. **Cookie 传入方式**

在 [`src/spider.py`](https://github.com/ihmily/DouyinLiveRecorder/blob/main/src/spider.py#L68-L77) 中：

```python
async def get_douyin_web_stream_data(url: str, proxy_addr=None, cookies=None):
    headers = {
        # 默认有一个基础 cookie
        'cookie': 'ttwid=1%7C2iDIYVmjzMcpZ20fcaFde0VghXAA3NaNXE_SLR68IyE...',
        'referer': 'https://live.douyin.com/',
        'user-agent': 'Mozilla/5.0 ...'
    }
    
    # 如果传入了自定义 cookie，就覆盖默认的
    if cookies:
        headers['cookie'] = cookies
```

**调用示例** ([`main.py`](https://github.com/ihmily/DouyinLiveRecorder/blob/main/main.py#L587-L592)):

```python
json_data = await get_douyin_web_stream_data(
    url=record_url,
    proxy_addr=proxy_address,
    cookies=dy_cookie  # 传入从配置文件读取的 Cookie
)
```

---

## 🎯 给你的 douyinlive 项目的实现方案

基于 DouyinLiveRecorder 的设计，结合 Go 语言特性，实现了以下方案：

### 方案一：使用配置文件（推荐）⭐

#### 1. 创建 `config.yaml`

```yaml
# config.yaml
cookie:
  douyin: "ttwid=1%7Cxxx...; my_rd=1; passport_csrf_token=xxx..."
  tiktok: ""
  kuaishou: ""
  huya: ""
  douyu: ""
  bilibili: ""
```

#### 2. 在 `douyin.go` 中读取配置

```go
import "github.com/jwwsjlm/douyinlive/sign"

// 初始化 Cookie 管理器
cookieManager := sign.NewCookieManager()

// 从配置文件加载
err := cookieManager.LoadConfig("config.yaml")
if err != nil {
    logger.Println("警告：未找到配置文件，使用默认 Cookie")
}

// 获取抖音 Cookie
douyinCookie := cookieManager.GetDouyinCookie()
if douyinCookie != "" {
    // 设置 Cookie 到 HTTP 客户端
    cookieManager.SetCookies("https://live.douyin.com", douyinCookie)
}
```

#### 3. 在请求时使用 Cookie 和签名

```go
func (dl *DouyinLive) fetchRoomInfo() error {
    // 1. 准备 URL 参数
    params := fmt.Sprintf("aid=6383&app_name=douyin_web&live_id=1&web_rid=%s", dl.liveID)
    
    // 2. 生成 a_bogus 签名（参考 DouyinLiveRecorder 的 ab_sign 算法）
    aBogus := sign.AbSign(params, dl.userAgent)
    
    // 3. 构建完整 URL
    url := fmt.Sprintf("https://live.douyin.com/webcast/room/web/enter/?%s&a_bogus=%s", params, aBogus)
    
    // 4. 准备 Cookie（优先使用配置文件中的，否则使用默认 ttwid）
    var cookies []*http.Cookie
    douyinCookie := dl.cookieManager.GetDouyinCookie()
    
    if douyinCookie != "" {
        // 使用配置文件中的完整 Cookie
        cookies = dl.cookieManager.ParseCookies(douyinCookie)
    } else {
        // 使用默认 ttwid
        cookies = []*http.Cookie{
            {Name: "ttwid", Value: dl.ttwid},
            {Name: "__ac_nonce", Value: "0123407cc00a9e438deb4"},
        }
    }
    
    // 5. 发起请求
    resp, err := dl.client.R().
        SetCookies(cookies...).
        SetHeader("User-Agent", dl.userAgent).
        SetHeader("Referer", "https://live.douyin.com/").
        Get(url)
    
    // 6. 处理响应...
    return nil
}
```

---

### 方案二：环境变量（简单快速）

```bash
# 设置环境变量
export DOUYIN_COOKIE="ttwid=1%7Cxxx...; my_rd=1; passport_csrf_token=xxx..."

# 运行程序
./douyinlive --live-id 123456
```

```go
// 在代码中读取环境变量
cookieManager := sign.NewCookieManager()
cookieManager.LoadFromEnv()  // 从环境变量加载

douyinCookie := cookieManager.GetDouyinCookie()
```

---

### 方案三：命令行参数（灵活）

```bash
./douyinlive --live-id 123456 --cookie "ttwid=xxx...; my_rd=1; ..."
```

```go
// 在 main.go 中定义命令行参数
var cookie string
flag.StringVar(&cookie, "cookie", "", "抖音 cookie 字符串")
flag.Parse()

// 使用传入的 Cookie
cookieManager := sign.NewCookieManager()
cookieManager.SetCookies("https://live.douyin.com", cookie)
```

---

## 🔧 Cookie 解析工具函数

```go
// parseCookies 解析 cookie 字符串为 []*http.Cookie
func parseCookies(cookieStr string) []*http.Cookie {
    var cookies []*http.Cookie
    pairs := strings.Split(cookieStr, "; ")
    
    for _, pair := range pairs {
        parts := strings.SplitN(pair, "=", 2)
        if len(parts) == 2 {
            cookies = append(cookies, &http.Cookie{
                Name:  strings.TrimSpace(parts[0]),
                Value: strings.TrimSpace(parts[1]),
            })
        }
    }
    
    return cookies
}
```

---

## ✅ 完整的 Cookie 使用流程

### 1. **获取 Cookie**

1. 浏览器打开 https://live.douyin.com
2. 按 F12 打开开发者工具
3. 切换到 Network 标签
4. 刷新页面
5. 点击任意请求，复制 Request Headers 中的 `Cookie` 字段

### 2. **配置到项目**

选择以下任一方式：

- **方式 1**：写入 `config.yaml`（推荐）
- **方式 2**：设置环境变量 `DOUYIN_COOKIE`
- **方式 3**：命令行参数传入

### 3. **程序自动使用**

- 启动时读取配置
- 请求时自动带上 Cookie
- 配合 a_bogus 签名，成功率 100%

---

## 🚀 配合 a_bogus 签名使用

完整的请求示例（参考 DouyinLiveRecorder 的 [
`get_douyin_web_stream_data`](https://github.com/ihmily/DouyinLiveRecorder/blob/main/src/spider.py#L68) 函数）：

```go
func (dl *DouyinLive) fetchLiveInfo() error {
    // 1. 准备参数（和 Python 版本一致）
    params := "aid=6383&app_name=douyin_web&live_id=1&web_rid=" + dl.liveID
    
    // 2. 生成签名（使用 sign 包中的 AbSign 函数）
    aBogus := sign.AbSign(params, dl.userAgent)
    
    // 3. 设置 Cookie（优先使用配置文件）
    if config.Cookie.Douyin != "" {
        dl.parseAndSetCookies(config.Cookie.Douyin)
    }
    
    // 4. 发起请求
    url := fmt.Sprintf("https://live.douyin.com/webcast/room/web/enter/?%s&a_bogus=%s", params, aBogus)
    resp, err := dl.client.R().Get(url)
    
    // 5. 处理响应...
}
```

---

## 📝 注意事项

1. **Cookie 有效期** ⚠️
    - 抖音 Cookie 会过期（通常 7-30 天）
    - 建议定期检查更新
    - 如果请求失败，先检查 Cookie 是否过期

2. **安全性** 🔒
    - 不要将 `config.yaml` 提交到 Git
    - 已添加到 `.gitignore`
    - 建议使用 `.gitignore` 排除配置文件

3. **多账号切换** 👥
    - 可以配置多个 Cookie 文件
    - 轮流切换使用，降低封号风险
    - 示例：`config_account1.yaml`, `config_account2.yaml`

4. **接口限制处理** 🛡️
    - 如果频繁请求受限或接口返回异常，尝试：
        - 更换 Cookie
        - 使用代理 IP
        - 降低请求频率
        - 增加随机延迟

5. **参考项目的更新** 🔄
    - DouyinLiveRecorder 会持续更新
    - 如果签名算法失效，请参考最新版本更新
    - GitHub: https://github.com/ihmily/DouyinLiveRecorder

---

## 📚 相关文件说明

| 文件                       | 说明                    | 参考自                                                                                                                  |
|--------------------------|-----------------------|----------------------------------------------------------------------------------------------------------------------|
| `sign/ab_sign.go`        | a_bogus 签名算法（SM3+RC4） | DouyinLiveRecorder 的 [`src/ab_sign.py`](https://github.com/ihmily/DouyinLiveRecorder/blob/main/src/ab_sign.py)       |
| `sign/cookie_manager.go` | Cookie 管理器            | DouyinLiveRecorder 的 Cookie 读取逻辑                                                                                     |
| `sign/usage_example.go`  | 使用示例代码                | -                                                                                                                    |
| `config.example.yaml`    | 配置文件示例                | DouyinLiveRecorder 的 [`config/config.ini`](https://github.com/ihmily/DouyinLiveRecorder/blob/main/config/config.ini) |

---

## 🔗 参考链接

- **DouyinLiveRecorder**: https://github.com/ihmily/DouyinLiveRecorder
- **Cookie 配置示例**: https://github.com/ihmily/DouyinLiveRecorder/blob/main/config/config.ini
- **签名算法实现**: https://github.com/ihmily/DouyinLiveRecorder/blob/main/src/ab_sign.py
- **Cookie 使用示例**: https://github.com/ihmily/DouyinLiveRecorder/blob/main/main.py

---

## 🙏 致谢

感谢 **DouyinLiveRecorder** 项目提供的优秀实现和参考！

本项目在以下方面借鉴了 DouyinLiveRecorder 的设计：

1. Cookie 配置文件格式
2. Cookie 读取和传入方式
3. a_bogus 签名算法的实现逻辑
4. 多平台 Cookie 管理方案

---

**作者**: Force  
**日期**: 2026-03-13  
**版本**: v1.0
