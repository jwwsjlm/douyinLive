package generated

import (
	"errors"
	"google.golang.org/protobuf/reflect/protoreflect"
	"log"
	"os"
)

var messageFactories map[string]func() protoreflect.ProtoMessage

func init() {
	log.SetOutput(os.Stdout)
	messageFactories = newMessage
	log.Printf("注册消息类型到消息池，共注册%d种消息类型\n", len(messageFactories))
}

// GetMessageInstance 获取消息实例
func GetMessageInstance(name string) (protoreflect.ProtoMessage, error) {
	if factory, ok := messageFactories[name]; ok {
		return factory(), nil
	}
	return nil, errors.New("未知消息: " + name)
}
