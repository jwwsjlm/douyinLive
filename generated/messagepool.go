package generated

import (
	"errors"

	"google.golang.org/protobuf/reflect/protoreflect"
)

var messageFactories = newMessage

// GetMessageInstance 获取消息实例
func GetMessageInstance(name string) (protoreflect.ProtoMessage, error) {
	if factory, ok := messageFactories[name]; ok {
		return factory(), nil
	}
	return nil, errors.New("未知消息: " + name)
}
