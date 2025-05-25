package generated

import (
	"errors"
	"google.golang.org/protobuf/reflect/protoreflect"
	"log"
	"os"
	"sync"
)

// NewMessageSync 使用 sync.Map 替代普通 map
var NewMessageSync = &sync.Map{}
var syncOnce sync.Once

// 初始化函数
func init() {

	syncOnce.Do(

		func() {
			log.SetOutput(os.Stdout)
			//来源:https://github.com/qiaoruntao/douyin_contract/blob/master/mapping.json
			//https://github.com/Remember-the-past/douyin_proto/blob/main/method%E5%AF%B9%E5%BA%94proto%E5%85%B3%E7%B3%BB.md
			c := countSyncMap(NewMessage)
			log.Printf("注册消息类型到消息池，共注册%d种消息类型\n", c)

		})

}

// countSyncMap 计算sync.Map中的元素数量
func countSyncMap(m map[string]func() protoreflect.ProtoMessage) int {
	count := 0
	for k, v := range m {
		registerMessage(k, v)
		count++
	}
	return count
	//m.Range(func(key, value interface{}) bool {
	//
	//	count++
	//	return true
	//})
	//return count
}

// registerMessage 注册消息类型到消息池
// 注册消息类型
func registerMessage(name string, factory func() protoreflect.ProtoMessage) {
	NewMessageSync.Store(name, factory)
}

// GetMessageInstance 通过syncmap获取消息实例
func GetMessageInstance(name string) (protoreflect.ProtoMessage, error) {
	// 快速路径：使用 Load 避免 LoadOrStore 的额外开销
	if factory, ok := NewMessageSync.Load(name); ok {
		return factory.(func() protoreflect.ProtoMessage)(), nil
	}
	return nil, errors.New("未知消息: " + name)
}
