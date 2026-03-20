package sign

// 这是一个使用示例，展示如何在 douyin.go 中集成 Cookie 和签名
// 实际使用时，将下面的代码整合到你的 douyin.go 中

/*
// 在 douyin.go 的 DouyinLive 结构体中添加
type DouyinLive struct {
    liveID      string
    userAgent   string
    client      *req.Client
    ttwid       string
    roomID      string
    pushID      string
    LiveName    string
    // ... 其他字段

    // 新增：Cookie 管理器
    cookieManager *sign.CookieManager

    // ... 其他字段
}

// 在 NewDouyinLive 函数中初始化 Cookie 管理器
func NewDouyinLive(liveID string, logger logger) (*DouyinLive, error) {
    dl := &DouyinLive{
        liveID:    liveID,
        userAgent: utils.RandomUserAgent(),
        client:    req.C().SetUserAgent(utils.RandomUserAgent()),
        // ... 其他初始化
    }

    // 初始化 Cookie 管理器
    dl.cookieManager = sign.NewCookieManager()

    // 方式 1：从配置文件加载
    err := dl.cookieManager.LoadConfig("config.yaml")
    if err != nil {
        logger.Println("警告：未找到配置文件，使用默认 Cookie")
    }

    // 方式 2：从环境变量加载（优先级更高）
    dl.cookieManager.LoadFromEnv()

    // 获取抖音 Cookie
    douyinCookie := dl.cookieManager.GetDouyinCookie()
    if douyinCookie != "" {
        // 设置 Cookie 到 client
        dl.cookieManager.SetCookies("https://live.douyin.com", douyinCookie)
    }

    return dl, nil
}

// 修改 fetchRoomInfo 函数，使用 Cookie 和签名
func (dl *DouyinLive) fetchRoomInfo() error {
    // 1. 准备 URL 参数
    params := fmt.Sprintf("aid=6383&app_name=douyin_web&live_id=1&device_platform=web&web_rid=%s", dl.liveID)

    // 2. 生成 a_bogus 签名
    aBogus := sign.AbSign(params, dl.userAgent)

    // 3. 构建完整 URL
    url := fmt.Sprintf("https://live.douyin.com/webcast/room/web/enter/?%s&a_bogus=%s", params, aBogus)

    // 4. 准备 Cookie
    var cookies []*http.Cookie

    // 如果配置文件中有完整的 cookie，就用配置文件的
    douyinCookie := dl.cookieManager.GetDouyinCookie()
    if douyinCookie != "" {
        cookies = dl.cookieManager.ParseCookies(douyinCookie)
    } else {
        // 否则使用默认的 ttwid
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

    if err != nil {
        return fmt.Errorf("请求直播间信息失败：%w", err)
    }

    // 6. 处理响应
    // ...

    return nil
}

// 添加一个辅助函数，用于更新 Cookie
func (dl *DouyinLive) UpdateCookie(cookieStr string) error {
    dl.cookieManager.UpdateCookie("douyin", cookieStr)

    // 保存到配置文件（可选）
    return dl.cookieManager.SaveConfig("config.yaml")
}

// 添加一个辅助函数，用于检查 Cookie 是否有效
func (dl *DouyinLive) CheckCookieValid() bool {
    douyinCookie := dl.cookieManager.GetDouyinCookie()
    return dl.cookieManager.ValidateCookie(douyinCookie)
}
*/

// 使用说明：
// 1. 复制上面的代码到你的 douyin.go 文件中
// 2. 根据实际情况调整字段名和函数名
// 3. 创建 config.yaml 文件，填入你的 Cookie
// 4. 运行程序，会自动使用配置的 Cookie 和 a_bogus 签名

// 完整的调用流程：
// 1. 程序启动 -> 加载 config.yaml -> 初始化 CookieManager
// 2. 发起请求 -> 生成 a_bogus 签名 -> 设置 Cookie -> 请求 API
// 3. 如果 Cookie 过期 -> 手动更新 Cookie -> 保存到 config.yaml
