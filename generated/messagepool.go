package generated

import (
	"errors"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

var messageFactories = newMessage
var messagePools sync.Map

func getMessagePool(name string) (*sync.Pool, bool) {
	if pool, ok := messagePools.Load(name); ok {
		return pool.(*sync.Pool), true
	}

	factory, ok := messageFactories[name]
	if !ok {
		return nil, false
	}

	pool := &sync.Pool{
		New: func() any {
			return factory()
		},
	}
	actual, _ := messagePools.LoadOrStore(name, pool)
	return actual.(*sync.Pool), true
}

// GetMessageInstance 获取消息实例
func GetMessageInstance(name string) (protoreflect.ProtoMessage, error) {
	pool, ok := getMessagePool(name)
	if !ok {
		return nil, errors.New("未知消息: " + name)
	}
	msg := pool.Get().(protoreflect.ProtoMessage)
	proto.Reset(msg)
	return msg, nil
}

// PutMessageInstance 回收消息实例
func PutMessageInstance(name string, msg protoreflect.ProtoMessage) {
	pool, ok := getMessagePool(name)
	if !ok || msg == nil {
		return
	}
	proto.Reset(msg)
	pool.Put(msg)
}
