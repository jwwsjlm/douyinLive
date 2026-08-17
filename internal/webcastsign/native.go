// Package webcastsign implements the compact signature used by Douyin's webcast WebSocket handshake.
package webcastsign

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/rc4"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"sync"
)

const (
	websocketModeFlag = byte(1 << 6)
	environmentCode   = byte(1)
	userBehaviorCode  = byte(14)
	customAlphabet    = "Dkdpgh4ZKsQB80/Mfvw36XI1R25+WUAlEi7NLboqYTOPuzmFjJnryx9HVGcaStCe"
)

var (
	// ErrInvalidXMSStub 表示输入不是 16 字节 MD5 十六进制字符串。
	// ErrInvalidXMSStub indicates that the input is not a 16-byte MD5 hexadecimal string.
	ErrInvalidXMSStub = errors.New("X-MS-STUB 必须是 32 位十六进制 MD5")

	websocketEncoding = base64.NewEncoding(customAlphabet).WithPadding(base64.NoPadding)
	emptyBodyDigest   = doubleMD5(nil)
)

// Generator 保存单个直播会话的签名序号和随机源。
// Generator owns the signature sequence and random source for one live session.
type Generator struct {
	mu      sync.Mutex
	counter byte
	random  io.Reader
}

// NewGenerator 创建使用系统安全随机源的原生 WebSocket 签名生成器。
// NewGenerator creates a native WebSocket signature generator backed by the system random source.
func NewGenerator() *Generator {
	return NewGeneratorWithReader(rand.Reader)
}

// NewGeneratorWithReader 创建使用指定随机源的生成器，主要用于确定性测试。
// NewGeneratorWithReader creates a generator with the supplied random source, primarily for deterministic tests.
func NewGeneratorWithReader(random io.Reader) *Generator {
	if random == nil {
		random = rand.Reader
	}
	return &Generator{random: random}
}

// Sign 根据 X-MS-STUB 生成 16 字符 WebSocket signature。
// Sign generates the 16-character WebSocket signature for an X-MS-STUB value.
func (g *Generator) Sign(xMSStub string) (string, error) {
	if g == nil {
		return "", errors.New("native WebSocket signer is nil")
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	flagRandom, err := readRandomByte(g.random, false)
	if err != nil {
		return "", err
	}
	g.counter++
	counter := g.counter & 0x3f
	payloadRandom, err := readRandomByte(g.random, true)
	if err != nil {
		return "", err
	}
	keyRandom, err := readRandomByte(g.random, true)
	if err != nil {
		return "", err
	}

	return SignWithValues(xMSStub, counter, flagRandom&1 == 1, payloadRandom, keyRandom)
}

// SignWithValues 使用显式状态值生成签名，以便与原始 JavaScript 做逐字节差异测试。
// SignWithValues signs with explicit state values for byte-for-byte differential testing against JavaScript.
// 此函数位于 internal 包中，不属于项目的公开兼容 API。
// This function lives in an internal package and is not part of the project's public compatibility API.
func SignWithValues(xMSStub string, counter byte, randomFlag bool, payloadRandom, keyRandom byte) (string, error) {
	stubBytes, err := hex.DecodeString(xMSStub)
	if err != nil || len(stubBytes) != md5.Size {
		return "", ErrInvalidXMSStub
	}
	stubDigest := md5.Sum(stubBytes)

	payload := [10]byte{
		counter & 0x3f,
		0,
		environmentCode,
		userBehaviorCode,
		emptyBodyDigest[14],
		emptyBodyDigest[15],
		stubDigest[14],
		stubDigest[15],
		payloadRandom,
	}
	for index := 0; index < len(payload)-1; index++ {
		payload[len(payload)-1] ^= payload[index]
	}

	cipher, err := rc4.NewCipher([]byte{keyRandom})
	if err != nil {
		return "", err
	}
	encrypted := make([]byte, len(payload))
	cipher.XORKeyStream(encrypted, payload[:])

	flags := websocketModeFlag
	if randomFlag {
		flags |= 1 << 4
	}
	result := make([]byte, 0, 2+len(encrypted))
	result = append(result, flags, keyRandom)
	result = append(result, encrypted...)
	return websocketEncoding.EncodeToString(result), nil
}

// doubleMD5 计算原始摘要字节再次参与 MD5 的双重摘要。
// doubleMD5 hashes data and then hashes the raw first digest once more.
func doubleMD5(data []byte) [md5.Size]byte {
	first := md5.Sum(data)
	return md5.Sum(first[:])
}

// readRandomByte 读取随机字节；excludeFF=true 时保持与 Math.floor(255*Math.random()) 的 0-254 范围一致。
// readRandomByte reads a random byte; excludeFF=true preserves Math.floor(255*Math.random())'s 0-254 range.
func readRandomByte(reader io.Reader, excludeFF bool) (byte, error) {
	var value [1]byte
	for {
		if _, err := io.ReadFull(reader, value[:]); err != nil {
			return 0, err
		}
		if !excludeFF || value[0] != 0xff {
			return value[0], nil
		}
	}
}
