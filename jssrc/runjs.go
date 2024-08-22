package jssrc

import (
	_ "embed"
	"github.com/dop251/goja"
)

// js代码来源:https://github.com/hua0512/stream-rec/blob/f5a13e5ccc7df051b4f537321ffa259275aaa1ba/platforms/src/main/resources/douyin-webmssdk.js
// https://github.com/LyzenX/DouyinLiveRecorder/blob/main/dylr/util/webmssdk.js
// 感谢前辈们做出的贡献
//
//go:embed webmssdk.js
var jsstr string
var vm *goja.Runtime
var fGetSing func(string) string

// LoadGoja 加载js到func当中
func LoadGoja(ua string) error {
	var err error
	// 创建一个新的Goja VM
	vm = goja.New()
	jsdom := "navigator = {" +
		"userAgent: '" + ua + "'" + "};" +
		"window=this;" +
		"document ={};" +
		"window.navigator = navigator;" +
		"setTimeout=function(){};"

	_, err = vm.RunString(jsdom + jsstr)
	//
	if err != nil {
		return err
	}
	//
	err = vm.ExportTo(vm.Get("get_sign"), &fGetSing)
	return err
}
func ExecuteJS(signature string) string {
	return fGetSing(signature)
}
