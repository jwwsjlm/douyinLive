package utils

import (
	"crypto/rand"
	"math/big"
)

func GenerateMsToken(length int) string {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+="

	b := make([]byte, length)
	for i := 0; i < length; i++ {
		// 生成0到charset长度之间的随机数
		randInt, _ := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))

		// 将随机数转换为字符集中的字符
		b[i] = charset[randInt.Int64()]
	}

	return string(b) + "=_"
}
