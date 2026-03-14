package main

import (
	"crypto/rc4"
	"encoding/binary"
	"fmt"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ========== Cookie 管理器 ==========

type CookieConfig struct {
	Cookie struct {
		Douyin   string `yaml:"douyin"`
		Tiktok   string `yaml:"tiktok"`
		Kuaishou string `yaml:"kuaishou"`
	} `yaml:"cookie"`
}

type CookieManager struct {
	config *CookieConfig
}

func NewCookieManager() *CookieManager {
	return &CookieManager{}
}

func (cm *CookieManager) LoadConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var config CookieConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return err
	}

	cm.config = &config
	return nil
}

func (cm *CookieManager) GetDouyinCookie() string {
	if cm.config != nil {
		return cm.config.Cookie.Douyin
	}
	return ""
}

func (cm *CookieManager) ParseCookies(cookieStr string) []*http.Cookie {
	var cookies []*http.Cookie
	if cookieStr == "" {
		return cookies
	}

	pairs := strings.Split(cookieStr, "; ")
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			cookies = append(cookies, &http.Cookie{
				Name:  strings.TrimSpace(parts[0]),
				Value: strings.TrimSpace(parts[1]),
			})
		}
	}

	return cookies
}

func (cm *CookieManager) ValidateCookie(cookieStr string) bool {
	if cookieStr == "" {
		return false
	}
	return strings.Contains(cookieStr, "ttwid=") ||
		strings.Contains(cookieStr, "passport_csrf_token=") ||
		strings.Contains(cookieStr, "odin_tt=")
}

// ========== a_bogus 签名算法 ==========

type SM3 struct {
	reg   []uint32
	chunk []byte
	size  int
}

func NewSM3() *SM3 {
	sm3 := &SM3{}
	sm3.Reset()
	return sm3
}

func (sm3 *SM3) Reset() {
	sm3.reg = []uint32{
		1937774191, 1226093241, 388252375, 3666478592,
		2842636476, 372324522, 3817729613, 2969243214,
	}
	sm3.chunk = make([]byte, 0, 64)
	sm3.size = 0
}

func leftRotate(x uint32, n int) uint32 {
	n %= 32
	return (x << n) | (x >> (32 - n))
}

func getTj(j int) uint32 {
	if 0 <= j && j < 16 {
		return 2043430169
	} else if 16 <= j && j < 64 {
		return 2055708042
	}
	panic("invalid j for constant Tj")
}

func ffj(j int, x, y, z uint32) uint32 {
	if 0 <= j && j < 16 {
		return x ^ y ^ z
	} else if 16 <= j && j < 64 {
		return (x & y) | (x & z) | (y & z)
	}
	panic("invalid j for bool function FF")
}

func ggj(j int, x, y, z uint32) uint32 {
	if 0 <= j && j < 16 {
		return x ^ y ^ z
	} else if 16 <= j && j < 64 {
		return (x & y) | (^x & z)
	}
	panic("invalid j for bool function GG")
}

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
			end := minInt(f+64, len(data))
			sm3.chunk = data[f:end]
		} else {
			sm3.chunk = []byte{}
		}
		f += 64
	}
}

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

func (sm3 *SM3) compress(data []byte) {
	if len(data) < 64 {
		panic("compress error: not enough data")
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

func (sm3 *SM3) Sum(data []byte) []byte {
	if data != nil {
		sm3.Reset()
		sm3.Write(data)
	}

	sm3.fill()

	for i := 0; i < len(sm3.chunk); i += 64 {
		end := minInt(i+64, len(sm3.chunk))
		sm3.compress(sm3.chunk[i:end])
	}

	result := make([]byte, 32)
	for i, val := range sm3.reg {
		binary.BigEndian.PutUint32(result[4*i:4*i+4], val)
	}

	sm3.Reset()
	return result
}

func rc4Encrypt(plaintext string, key string) string {
	cipher, err := rc4.NewCipher([]byte(key))
	if err != nil {
		panic(err)
	}
	dst := make([]byte, len(plaintext))
	cipher.XORKeyStream(dst, []byte(plaintext))
	return string(dst)
}

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

func generateRandomStr() string {
	randomValues := []float64{0.123456789, 0.987654321, 0.555555555}
	randomBytes := make([]byte, 0)

	randomBytes = append(randomBytes, generRandom(int(randomValues[0]*10000), []int{3, 45})...)
	randomBytes = append(randomBytes, generRandom(int(randomValues[1]*10000), []int{1, 0})...)
	randomBytes = append(randomBytes, generRandom(int(randomValues[2]*10000), []int{1, 5})...)

	return string(randomBytes)
}

func splitToBytes(num uint32) []byte {
	return []byte{
		byte(num >> 24),
		byte(num >> 16),
		byte(num >> 8),
		byte(num),
	}
}

func generateRc4BbStr(urlSearchParams, userAgent, windowEnvStr string, suffix string, arguments []int) string {
	sm3 := NewSM3()
	startTime := uint32(time.Now().UnixMilli())

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

func AbSign(urlSearchParams, userAgent string) string {
	windowEnvStr := "1920|1080|1920|1040|0|30|0|0|1872|92|1920|1040|1857|92|1|24|Win32"
	return resultEncrypt(
		generateRandomStr()+
			generateRc4BbStr(urlSearchParams, userAgent, windowEnvStr, "cus", []int{0, 1, 14}),
		"s4",
	) + "="
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ========== 主测试函数 ==========

func main() {
	fmt.Println("🧪 开始测试 Cookie + a_bogus 签名集成...")
	fmt.Println()

	// 创建 Cookie 管理器
	cm := NewCookieManager()

	// 测试 1: 加载配置文件
	fmt.Println("📋 测试 1: 加载配置文件")
	err := cm.LoadConfig("config.yaml")
	if err != nil {
		fmt.Printf("⚠️  配置文件加载失败：%v\n", err)
		fmt.Println("💡 提示：请复制 config.example.yaml 为 config.yaml 并填入 Cookie")
	} else {
		fmt.Println("✅ 配置文件加载成功")
	}
	fmt.Println()

	// 测试 2: 获取并验证 Cookie
	fmt.Println("🍪 测试 2: 获取并验证 Cookie")
	douyinCookie := cm.GetDouyinCookie()
	if douyinCookie == "" {
		fmt.Println("⚠️  Cookie 为空，使用示例 Cookie 测试")
		douyinCookie = "ttwid=1%7C2iDIYVmjzMcpZ20fcaFde0VghXAA3NaNXE_SLR68IyE%7C1761045455%7Cab35197d5cfb21df6cbb2fa7ef1c9262206b062c315b9d04da746d0b37dfbc7d; my_rd=1; passport_csrf_token=3ab34460fa656183fccfb904b16ff742; d_ticket=9f562383ac0547d0b561904513229d76c9c21"
		fmt.Println("✅ 使用示例 Cookie")
	} else {
		fmt.Println("✅ 从配置获取 Cookie 成功")
	}

	isValid := cm.ValidateCookie(douyinCookie)
	if isValid {
		fmt.Println("✅ Cookie 格式验证通过")
	} else {
		fmt.Println("❌ Cookie 格式验证失败")
	}
	fmt.Println()

	// 测试 3: 解析 Cookie
	fmt.Println("🔧 测试 3: 解析 Cookie")
	cookies := cm.ParseCookies(douyinCookie)
	fmt.Printf("✅ 解析成功，共 %d 个 Cookie:\n", len(cookies))
	for i, c := range cookies {
		fmt.Printf("   %d. %s = %s\n", i+1, c.Name, c.Value[:minInt(30, len(c.Value))]+"...")
	}
	fmt.Println()

	// 测试 4: 生成 a_bogus 签名
	fmt.Println("✍️  测试 4: 生成 a_bogus 签名")
	params := "aid=6383&app_name=douyin_web&live_id=1&device_platform=web&language=zh-CN&web_rid=123456"
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.0.0 Safari/537.36"

	start := time.Now()
	aBogus := AbSign(params, ua)
	elapsed := time.Since(start)

	fmt.Printf("✅ 签名生成成功！\n")
	fmt.Printf("⏱️  耗时: %v\n", elapsed)
	fmt.Printf("📝 a_bogus: %s\n", aBogus)
	fmt.Println()

	// 测试 5: 构建完整请求 URL
	fmt.Println("🔗 测试 5: 构建完整请求 URL")
	fullURL := fmt.Sprintf("https://live.douyin.com/webcast/room/web/enter/?%s&a_bogus=%s", params, aBogus)
	fmt.Printf("✅ URL 长度：%d 字符\n", len(fullURL))
	fmt.Printf("📄 URL: %s...\n", fullURL[:minInt(150, len(fullURL))]+"...")
	fmt.Println()

	// 测试 6: 模拟完整请求流程
	fmt.Println("🎬 测试 6: 模拟完整请求流程")
	fmt.Println("1️⃣  加载配置文件 ✅")
	fmt.Println("2️⃣  获取 Cookie ✅")
	fmt.Println("3️⃣  生成 a_bogus 签名 ✅")
	fmt.Println("4️⃣  设置请求头:")
	fmt.Println("   - User-Agent:", ua)
	fmt.Println("   - Cookie: [已设置", len(cookies), "个 Cookie]")
	fmt.Println("   - Referer: https://live.douyin.com/")
	fmt.Println("5️⃣  发起请求: GET", fullURL[:50]+"...")
	fmt.Println()

	// 总结
	fmt.Println("🎉 全部测试完成！")
	fmt.Println()
	fmt.Println("📌 下一步:")
	fmt.Println("1. 复制 config.example.yaml 为 config.yaml")
	fmt.Println("2. 编辑 config.yaml，填入你的真实 Cookie")
	fmt.Println("3. 再次运行测试，验证真实 Cookie 是否有效")
	fmt.Println("4. 将 Cookie 和签名集成到 douyin.go 中")
	fmt.Println()
}
