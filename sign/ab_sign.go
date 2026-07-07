package sign

import (
	"crypto/rc4"
	"encoding/binary"
	"math"
	"time"
)

// SM3 实现国密 SM3 哈希算法的内部状态。
// SM3 stores the internal state for the SM3 hash implementation.
type SM3 struct {
	reg   []uint32
	chunk []byte
	size  int
}

// NewSM3 创建并初始化 SM3 哈希器。
// NewSM3 creates and initializes an SM3 hasher.
func NewSM3() *SM3 {
	sm3 := &SM3{}
	sm3.Reset()
	return sm3
}

// Reset 将 SM3 哈希器恢复到初始状态。
// Reset restores the SM3 hasher to its initial state.
func (sm3 *SM3) Reset() {
	sm3.reg = []uint32{
		1937774191, 1226093241, 388252375, 3666478592,
		2842636476, 372324522, 3817729613, 2969243214,
	}
	sm3.chunk = make([]byte, 0, 64)
	sm3.size = 0
}

// leftRotate 对 uint32 执行循环左移。
// leftRotate rotates a uint32 value left by n bits.
// 参数/Parameters:
//   - x: 待旋转的 32 位整数。 32-bit integer to rotate.
//   - n: 左移位数。 Number of bits to rotate left.
func leftRotate(x uint32, n int) uint32 {
	n %= 32
	return (x << n) | (x >> (32 - n))
}

// getTj 返回 SM3 压缩函数第 j 轮常量。
// getTj returns the round constant for step j of the SM3 compression function.
// 参数/Parameters:
//   - j: SM3 压缩轮次，范围为 0-63。 SM3 compression round index, in the 0-63 range.
func getTj(j int) uint32 {
	if 0 <= j && j < 16 {
		return 2043430169 // 0x79CC4519
	} else if 16 <= j && j < 64 {
		return 2055708042 // 0x7A879D8A
	}
	// j 应该在 0-63 范围内，这是内部不变量
	return 0
}

// ffj 执行 SM3 压缩函数中的 FF 布尔函数。
// ffj evaluates the FF boolean function used by SM3 compression.
// 参数/Parameters:
//   - j: SM3 压缩轮次。 SM3 compression round index.
//   - x: FF 输入字。 FF input word.
//   - y: FF 输入字。 FF input word.
//   - z: FF 输入字。 FF input word.
func ffj(j int, x, y, z uint32) uint32 {
	if 0 <= j && j < 16 {
		return x ^ y ^ z
	} else if 16 <= j && j < 64 {
		return (x & y) | (x & z) | (y & z)
	}
	// j 应该在 0-63 范围内，这是内部不变量
	return 0
}

// ggj 执行 SM3 压缩函数中的 GG 布尔函数。
// ggj evaluates the GG boolean function used by SM3 compression.
// 参数/Parameters:
//   - j: SM3 压缩轮次。 SM3 compression round index.
//   - x: GG 输入字。 GG input word.
//   - y: GG 输入字。 GG input word.
//   - z: GG 输入字。 GG input word.
func ggj(j int, x, y, z uint32) uint32 {
	if 0 <= j && j < 16 {
		return x ^ y ^ z
	} else if 16 <= j && j < 64 {
		return (x & y) | (^x & z)
	}
	// j 应该在 0-63 范围内，这是内部不变量
	return 0
}

// Write 将数据追加到 SM3 哈希器并压缩完整分组。
// Write appends data to the SM3 hasher and compresses complete blocks.
// 参数/Parameters:
//   - data: 要写入哈希器的数据。 Data to write into the hasher.
func (sm3 *SM3) Write(data []byte) {
	sm3.size += len(data)
	f := 64 - len(sm3.chunk)

	if len(data) < f {
		sm3.chunk = append(sm3.chunk, data...)
		return
	}

	sm3.chunk = append(sm3.chunk, data[:f]...)
	for len(sm3.chunk) >= 64 {
		sm3.compress(sm3.chunk)
		if f < len(data) {
			end := min(f+64, len(data))
			sm3.chunk = data[f:end]
		} else {
			sm3.chunk = []byte{}
		}
		f += 64
	}
}

// fill 对剩余数据执行 SM3 填充。
// fill applies SM3 padding to the remaining data.
func (sm3 *SM3) fill() {
	bitLength := uint64(sm3.size) * 8
	sm3.chunk = append(sm3.chunk, 0x80)

	paddingPos := len(sm3.chunk) % 64
	if 64-paddingPos < 8 {
		for len(sm3.chunk)%64 != 0 {
			sm3.chunk = append(sm3.chunk, 0)
		}
	}

	for len(sm3.chunk)%64 < 56 {
		sm3.chunk = append(sm3.chunk, 0)
	}

	highBits := uint32(bitLength >> 32)
	sm3.chunk = append(sm3.chunk, byte(highBits>>24), byte(highBits>>16), byte(highBits>>8), byte(highBits))

	lowBits := uint32(bitLength & 0xFFFFFFFF)
	sm3.chunk = append(sm3.chunk, byte(lowBits>>24), byte(lowBits>>16), byte(lowBits>>8), byte(lowBits))
}

// compress 执行单个 SM3 512 位分组压缩。
// compress processes one 512-bit SM3 block.
// 参数/Parameters:
//   - data: 至少 64 字节的分组数据。 Block data with at least 64 bytes.
func (sm3 *SM3) compress(data []byte) {
	if len(data) < 64 {
		// 数据不足 64 字节，内部错误，返回
		// Data is shorter than 64 bytes; treat it as an internal error and return.
		return
	}

	w := make([]uint32, 132)
	for t := 0; t < 16; t++ {
		w[t] = binary.BigEndian.Uint32(data[4*t : 4*t+4])
	}

	for j := 16; j < 68; j++ {
		a := w[j-16] ^ w[j-9] ^ leftRotate(w[j-3], 15)
		a = a ^ leftRotate(a, 15) ^ leftRotate(a, 23)
		w[j] = (a ^ leftRotate(w[j-13], 7) ^ w[j-6])
	}

	for j := 0; j < 64; j++ {
		w[j+68] = w[j] ^ w[j+4]
	}

	a, b, c, d, e, f, g, h := sm3.reg[0], sm3.reg[1], sm3.reg[2], sm3.reg[3], sm3.reg[4], sm3.reg[5], sm3.reg[6], sm3.reg[7]

	for j := 0; j < 64; j++ {
		ss1 := leftRotate((leftRotate(a, 12) + e + leftRotate(getTj(j), j)), 7)
		ss2 := ss1 ^ leftRotate(a, 12)
		tt1 := (ffj(j, a, b, c) + d + ss2 + w[j+68])
		tt2 := (ggj(j, e, f, g) + h + ss1 + w[j])

		d = c
		c = leftRotate(b, 9)
		b = a
		a = tt1
		h = g
		g = leftRotate(f, 19)
		f = e
		e = (tt2 ^ leftRotate(tt2, 9) ^ leftRotate(tt2, 17))
	}

	sm3.reg[0] ^= a
	sm3.reg[1] ^= b
	sm3.reg[2] ^= c
	sm3.reg[3] ^= d
	sm3.reg[4] ^= e
	sm3.reg[5] ^= f
	sm3.reg[6] ^= g
	sm3.reg[7] ^= h
}

// Sum 返回数据的 SM3 摘要，并在完成后重置哈希器。
// Sum returns the SM3 digest for data and resets the hasher afterwards.
// 参数/Parameters:
//   - data: 要计算摘要的数据。 Data to hash.
func (sm3 *SM3) Sum(data []byte) []byte {
	if data != nil {
		sm3.Reset()
		sm3.Write(data)
	}

	sm3.fill()

	for i := 0; i < len(sm3.chunk); i += 64 {
		end := min(i+64, len(sm3.chunk))
		sm3.compress(sm3.chunk[i:end])
	}

	result := make([]byte, 32)
	for i, val := range sm3.reg {
		binary.BigEndian.PutUint32(result[4*i:4*i+4], val)
	}

	sm3.Reset()
	return result
}

// rc4Encrypt 使用 RC4 对文本进行异或流加密。
// rc4Encrypt encrypts text with RC4 stream encryption.
// 参数/Parameters:
//   - plaintext: 待加密文本。 Text to encrypt.
//   - key: RC4 密钥。 RC4 key.
func rc4Encrypt(plaintext string, key string) string {
	cipher, err := rc4.NewCipher([]byte(key))
	if err != nil {
		// RC4 密钥无效，返回空字符串
		return ""
	}
	dst := make([]byte, len(plaintext))
	cipher.XORKeyStream(dst, []byte(plaintext))
	return string(dst)
}

// resultEncrypt 使用指定编码表对二进制字符串进行变体 Base64 编码。
// resultEncrypt encodes a binary string with the selected custom Base64 table.
// 参数/Parameters:
//   - longStr: 二进制字符串内容。 Binary string content.
//   - num: 编码表选择编号。 Encoding table selector.
func resultEncrypt(longStr string, num string) string {
	encodingTables := map[string]string{
		"s0": "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/=",
		"s1": "Dkdpgh4ZKsQB80/Mfvw36XI1R25+WUAlEi7NLboqYTOPuzmFjJnryx9HVGcaStCe=",
		"s2": "Dkdpgh4ZKsQB80/Mfvw36XI1R25-WUAlEi7NLboqYTOPuzmFjJnryx9HVGcaStCe=",
		"s3": "ckdp1h4ZKsUB80/Mfvw36XIgR25+WQAlEi7NLboqYTOPuzmFjJnryx9HVGDaStCe",
		"s4": "Dkdpgh2ZmsQB80/MfvV36XI1R45-WUAlEixNLwoqYTOPuzKFjJnry79HbGcaStCe",
	}

	masks := []uint32{16515072, 258048, 4032, 63}
	shifts := []int{18, 12, 6, 0}
	encodingTable := encodingTables[num]

	result := make([]byte, 0)
	roundNum := 0
	longInt := getLongInt(roundNum, longStr)

	totalChars := int(math.Ceil(float64(len(longStr)) / 3 * 4))

	for i := 0; i < totalChars; i++ {
		if i/4 != roundNum {
			roundNum++
			longInt = getLongInt(roundNum, longStr)
		}

		index := i % 4
		charIndex := (longInt & masks[index]) >> shifts[index]
		result = append(result, encodingTable[charIndex])
	}

	return string(result)
}

// getLongInt 从字符串指定三字节分组中读取 24 位整数。
// getLongInt reads a 24-bit integer from a three-byte group in the string.
// 参数/Parameters:
//   - roundNum: 三字节分组序号。 Three-byte group index.
//   - longStr: 提供字节数据的字符串。 String that provides byte data.
func getLongInt(roundNum int, longStr string) uint32 {
	roundNum = roundNum * 3
	var char1, char2, char3 byte
	if roundNum < len(longStr) {
		char1 = longStr[roundNum]
	}
	if roundNum+1 < len(longStr) {
		char2 = longStr[roundNum+1]
	}
	if roundNum+2 < len(longStr) {
		char3 = longStr[roundNum+2]
	}
	return uint32(char1)<<16 | uint32(char2)<<8 | uint32(char3)
}

// generRandom 根据随机数和选项生成混淆字节。
// generRandom derives obfuscated bytes from a random number and option bytes.
// 参数/Parameters:
//   - randomNum: 输入随机数。 Input random number.
//   - option: 混淆选项字节。 Obfuscation option bytes.
func generRandom(randomNum int, option []int) []byte {
	byte1 := byte(randomNum & 255)
	byte2 := byte((randomNum >> 8) & 255)

	return []byte{
		(byte1 & 170) | (byte(option[0]) & 85),
		(byte1 & 85) | (byte(option[0]) & 170),
		(byte2 & 170) | (byte(option[1]) & 85),
		(byte2 & 85) | (byte(option[1]) & 170),
	}
}

// generateRandomStr 生成 a_bogus 前缀所需的固定随机片段。
// generateRandomStr generates the fixed pseudo-random prefix used by a_bogus.
func generateRandomStr() string {
	randomValues := []float64{0.123456789, 0.987654321, 0.555555555}
	randomBytes := make([]byte, 0)

	randomBytes = append(randomBytes, generRandom(int(randomValues[0]*10000), []int{3, 45})...)
	randomBytes = append(randomBytes, generRandom(int(randomValues[1]*10000), []int{1, 0})...)
	randomBytes = append(randomBytes, generRandom(int(randomValues[2]*10000), []int{1, 5})...)

	return string(randomBytes)
}

// splitToBytes 将 uint32 按大端序拆成四个字节。
// splitToBytes splits a uint32 into four big-endian bytes.
// 参数/Parameters:
//   - num: 待拆分的 32 位整数。 32-bit integer to split.
func splitToBytes(num uint32) []byte {
	return []byte{
		byte(num >> 24),
		byte(num >> 16),
		byte(num >> 8),
		byte(num),
	}
}

// generateRc4BbStr 组装 a_bogus 的加密 bb 字节串。
// generateRc4BbStr builds the encrypted bb byte string used by a_bogus.
// 参数/Parameters:
//   - urlSearchParams: 请求 URL 查询参数字符串。 Request URL query string.
//   - userAgent: 浏览器 User-Agent。 Browser User-Agent.
//   - windowEnvStr: 浏览器环境摘要字符串。 Browser environment summary string.
//   - suffix: a_bogus 后缀随机片段。 Random suffix fragment for a_bogus.
//   - arguments: 算法固定参数表。 Fixed algorithm argument table.
func generateRc4BbStr(urlSearchParams, userAgent, windowEnvStr string, suffix string, arguments []int) string {
	sm3 := NewSM3()
	startTime := uint32(time.Now().UnixMilli())

	// 计算三次哈希。
	// Compute the three hash rounds.
	urlHash := sm3.Sum(sm3.Sum([]byte(urlSearchParams + suffix)))
	cusHash := sm3.Sum(sm3.Sum([]byte(suffix)))
	uaKey := string([]byte{0, 1, 14})
	uaEncrypted := rc4Encrypt(userAgent, uaKey)
	uaEncoded := resultEncrypt(uaEncrypted, "s3")
	uaHash := sm3.Sum([]byte(uaEncoded))

	endTime := startTime + 100

	b := make(map[int]interface{})
	b[8] = 3
	b[10] = endTime
	b[15] = map[string]interface{}{
		"aid":    6383,
		"pageId": 110624,
		"boe":    false,
		"ddrt":   7,
		"paths": map[string]interface{}{
			"include": make([]map[string]interface{}, 7),
			"exclude": []interface{}{},
		},
		"track": map[string]interface{}{
			"mode":  0,
			"delay": 300,
			"paths": []interface{}{},
		},
		"dump": true,
		"rpU":  "hwj",
	}
	b[16] = startTime
	b[18] = 44
	b[19] = []int{1, 0, 1, 5}

	startTimeBytes := splitToBytes(b[16].(uint32))
	b[20] = startTimeBytes[0]
	b[21] = startTimeBytes[1]
	b[22] = startTimeBytes[2]
	b[23] = startTimeBytes[3]
	b[24] = byte(uint64(b[16].(uint32)) / 256 / 256 / 256 / 256 & 255)
	b[25] = byte(uint64(b[16].(uint32)) / 256 / 256 / 256 / 256 / 256 & 255)

	arg0Bytes := splitToBytes(uint32(arguments[0]))
	b[26] = arg0Bytes[0]
	b[27] = arg0Bytes[1]
	b[28] = arg0Bytes[2]
	b[29] = arg0Bytes[3]

	b[30] = byte(arguments[1] / 256 & 255)
	b[31] = byte(arguments[1] % 256 & 255)

	arg1Bytes := splitToBytes(uint32(arguments[1]))
	b[32] = arg1Bytes[0]
	b[33] = arg1Bytes[1]

	arg2Bytes := splitToBytes(uint32(arguments[2]))
	b[34] = arg2Bytes[0]
	b[35] = arg2Bytes[1]
	b[36] = arg2Bytes[2]
	b[37] = arg2Bytes[3]

	b[38] = urlHash[21]
	b[39] = urlHash[22]
	b[40] = cusHash[21]
	b[41] = cusHash[22]
	b[42] = uaHash[23]
	b[43] = uaHash[24]

	endTimeBytes := splitToBytes(b[10].(uint32))
	b[44] = endTimeBytes[0]
	b[45] = endTimeBytes[1]
	b[46] = endTimeBytes[2]
	b[47] = endTimeBytes[3]
	b[48] = b[8].(int)
	b[49] = byte(uint64(b[10].(uint32)) / 256 / 256 / 256 / 256 & 255)
	b[50] = byte(uint64(b[10].(uint32)) / 256 / 256 / 256 / 256 / 256 & 255)

	b[51] = b[15].(map[string]interface{})["pageId"].(int)
	pageIdBytes := splitToBytes(uint32(b[51].(int)))
	b[52] = pageIdBytes[0]
	b[53] = pageIdBytes[1]
	b[54] = pageIdBytes[2]
	b[55] = pageIdBytes[3]

	b[56] = b[15].(map[string]interface{})["aid"].(int)
	b[57] = byte(b[56].(int) & 255)
	b[58] = byte((b[56].(int) >> 8) & 255)
	b[59] = byte((b[56].(int) >> 16) & 255)
	b[60] = byte((b[56].(int) >> 24) & 255)

	windowEnvList := []byte(windowEnvStr)
	b[64] = len(windowEnvList)
	b[65] = byte(len(windowEnvList) & 255)
	b[66] = byte((len(windowEnvList) >> 8) & 255)

	b[69] = 0
	b[70] = 0
	b[71] = 0

	b[72] = b[18].(int) ^ int(b[20].(byte)) ^ int(b[26].(byte)) ^ int(b[30].(byte)) ^ int(b[38].(byte)) ^ int(b[40].(byte)) ^ int(b[42].(byte)) ^ int(b[21].(byte)) ^ int(b[27].(byte)) ^ int(b[31].(byte)) ^
		int(b[35].(byte)) ^ int(b[39].(byte)) ^ int(b[41].(byte)) ^ int(b[43].(byte)) ^ int(b[22].(byte)) ^ int(b[28].(byte)) ^ int(b[32].(byte)) ^ int(b[36].(byte)) ^ int(b[23].(byte)) ^ int(b[29].(byte)) ^
		int(b[33].(byte)) ^ int(b[37].(byte)) ^ int(b[44].(byte)) ^ int(b[45].(byte)) ^ int(b[46].(byte)) ^ int(b[47].(byte)) ^ int(b[48].(int)) ^ int(b[49].(byte)) ^ int(b[50].(byte)) ^ int(b[24].(byte)) ^
		int(b[25].(byte)) ^ int(b[52].(byte)) ^ int(b[53].(byte)) ^ int(b[54].(byte)) ^ int(b[55].(byte)) ^ int(b[57].(byte)) ^ int(b[58].(byte)) ^ int(b[59].(byte)) ^ int(b[60].(byte)) ^ int(b[65].(byte)) ^
		int(b[66].(byte)) ^ int(b[70].(int)) ^ int(b[71].(int))

	bb := []byte{
		byte(b[18].(int)), b[20].(byte), b[52].(byte), b[26].(byte), b[30].(byte), b[34].(byte), b[58].(byte), b[38].(byte), b[40].(byte), b[53].(byte), b[42].(byte), b[21].(byte),
		b[27].(byte), b[54].(byte), b[55].(byte), b[31].(byte), b[35].(byte), b[57].(byte), b[39].(byte), b[41].(byte), b[43].(byte), b[22].(byte), b[28].(byte), b[32].(byte),
		b[60].(byte), b[36].(byte), b[23].(byte), b[29].(byte), b[33].(byte), b[37].(byte), b[44].(byte), b[45].(byte), b[59].(byte), b[46].(byte), b[47].(byte), byte(b[48].(int)),
		b[49].(byte), b[50].(byte), b[24].(byte), b[25].(byte), b[65].(byte), b[66].(byte), byte(b[70].(int)), byte(b[71].(int)),
	}
	bb = append(bb, windowEnvList...)
	bb = append(bb, byte(b[72].(int)))

	bbStr := string(bb)
	return rc4Encrypt(bbStr, string([]byte{121}))
}

// AbSign 生成抖音 web/enter 请求所需的 a_bogus 签名。
// AbSign generates the a_bogus signature required by Douyin web/enter requests.
// 参数/Parameters:
//   - urlSearchParams: URL 查询参数字符串，例如 "aid=6383&app_name=douyin_web"。 URL query string, for example "aid=6383&app_name=douyin_web".
//   - userAgent: 浏览器 User-Agent。 Browser User-Agent.
func AbSign(urlSearchParams, userAgent string) string {
	windowEnvStr := "1920|1080|1920|1040|0|30|0|0|1872|92|1920|1040|1857|92|1|24|Win32"
	return resultEncrypt(
		generateRandomStr()+
			generateRc4BbStr(urlSearchParams, userAgent, windowEnvStr, "cus", []int{0, 1, 14}),
		"s4",
	) + "="
}
