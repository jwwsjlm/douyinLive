package jsScript

import (
	_ "embed"
	"sync"

	"github.com/dop251/goja"
)

// 嵌入的 JavaScript 文件来源于开源项目，感谢贡献者们的努力。
// The embedded JavaScript file comes from an open-source project; thanks to its contributors.
//
//go:embed webmssdk.js
var jsScript string

var (
	vm       *goja.Runtime
	fGetSign func(string) string
	mu       sync.Mutex
)

// LoadGoja 将 JavaScript 加载到 Goja 运行时，并设置签名所需的浏览器环境。
// LoadGoja loads JavaScript into the Goja runtime and prepares the browser-like signing environment.
func LoadGoja(ua string) error {
	mu.Lock()
	defer mu.Unlock()
	vm = goja.New()

	jsdom := `
		navigator = { userAgent: '` + ua + `' };
		window = this;
		document = {};
		window.navigator = navigator;
		setTimeout = function() {};
	`

	if _, err := vm.RunString(jsdom + jsScript); err != nil {
		return err
	}

	return vm.ExportTo(vm.Get("get_sign"), &fGetSign)
}

// ExecuteJS 调用 JavaScript 中的 get_sign 函数生成签名。
// ExecuteJS calls the JavaScript get_sign function to generate a signature.
func ExecuteJS(signature string) string {
	mu.Lock()
	defer mu.Unlock()
	return fGetSign(signature)
}
