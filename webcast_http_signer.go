package douyinLive

import (
	"fmt"
	"time"

	"github.com/jwwsjlm/douyinLive/v2/sign"
)

const webcastHTTPABogusProvider = "native_ab_sign"

// webcastHTTPSignResult 保存 HTTP webcast 请求的 a_bogus 签名结果和耗时。
// webcastHTTPSignResult stores the a_bogus result and timing for an HTTP webcast request.
type webcastHTTPSignResult struct {
	URL          string
	Provider     string
	ABogusLength int
	Duration     time.Duration
}

// signWebcastHTTPURL 使用轻量原生算法为 webcast HTTP URL 添加 a_bogus。
// signWebcastHTTPURL adds a_bogus to a webcast HTTP URL using the lightweight native algorithm.
// 参数/Parameters:
//   - endpoint: 不包含查询参数的接口地址。 Endpoint URL without query parameters.
//   - params: 保持浏览器参数顺序的原始查询字符串。 Raw query string preserving browser parameter order.
//   - userAgent: 参与签名的浏览器 User-Agent。 Browser User-Agent used by the signer.
func signWebcastHTTPURL(endpoint string, params string, userAgent string) webcastHTTPSignResult {
	startedAt := time.Now()
	aBogus := sign.AbSign(params, userAgent)
	return webcastHTTPSignResult{
		URL:          fmt.Sprintf("%s?%s&a_bogus=%s", endpoint, params, queryEscapeURLSearchParamsValue(aBogus)),
		Provider:     webcastHTTPABogusProvider,
		ABogusLength: len(aBogus),
		Duration:     time.Since(startedAt).Round(time.Microsecond),
	}
}
