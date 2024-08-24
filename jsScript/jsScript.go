package jsScript

import (
	_ "embed"
	"github.com/dop251/goja"
	"sync"
)

// 嵌入的 JavaScript 文件来源于开源项目，感谢贡献者们的努力
//
//go:embed webmssdk.js
var jsScript string

var (
	vm       *goja.Runtime
	fGetSign func(string) string
	mu       sync.Mutex
)

// LoadGoja 加载 JavaScript 到 Goja 运行时中，并设置必要的环境
func LoadGoja(ua string) error {
	mu.Lock()
	defer mu.Unlock()
	// 创建一个新的 Goja VM 实例
	vm = goja.New()

	// 构建 JavaScript 环境，模拟浏览器的 navigator 和 window 对象
	jsdom := `
		navigator = { userAgent: '` + ua + `' };
		window = this;
		document = {};
		window.navigator = navigator;
		setTimeout = function() {};
	`

	// 运行 JavaScript 环境设置和嵌入的 JavaScript 代码
	if _, err := vm.RunString(jsdom + jsScript); err != nil {
		return err
	}

	// 将 JavaScript 函数 get_sign 导出为 Go 函数 fGetSign
	return vm.ExportTo(vm.Get("get_sign"), &fGetSign)
}

// ExecuteJS 执行 JavaScript 中的 get_sign 函数
func ExecuteJS(signature string) string {
	mu.Lock()
	defer mu.Unlock()
	return fGetSign(signature)
}
