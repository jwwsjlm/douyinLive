package douyinLive

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"math/big"

	"github.com/jwwsjlm/douyinLive/v2/jsScript"
)

// browserFingerprint 保存一次直播会话内保持稳定、不同会话之间轮换的浏览器画像。
// browserFingerprint stores a coherent browser identity that is stable within a session and rotated between sessions.
type browserFingerprint struct {
	Preset              string
	ID                  string
	ScreenWidth         int
	ScreenHeight        int
	AvailWidth          int
	AvailHeight         int
	InnerWidth          int
	InnerHeight         int
	OuterWidth          int
	OuterHeight         int
	DevicePixelRatio    float64
	DeviceMemory        int
	HardwareConcurrency int
	WebGLVendor         string
	WebGLRenderer       string
	CanvasSeed          uint32
	RandomSeed          uint32
}

var browserFingerprintPresets = []browserFingerprint{
	{Preset: "win-fhd-rtx3060", ScreenWidth: 1920, ScreenHeight: 1080, AvailWidth: 1920, AvailHeight: 1040, InnerWidth: 1536, InnerHeight: 824, OuterWidth: 1552, OuterHeight: 919, DevicePixelRatio: 1, DeviceMemory: 16, HardwareConcurrency: 16, WebGLVendor: "Google Inc. (NVIDIA)", WebGLRenderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 3060 (0x00002504) Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{Preset: "win-fhd-gtx1080ti", ScreenWidth: 1920, ScreenHeight: 1080, AvailWidth: 1920, AvailHeight: 1032, InnerWidth: 1904, InnerHeight: 937, OuterWidth: 1920, OuterHeight: 1032, DevicePixelRatio: 1, DeviceMemory: 32, HardwareConcurrency: 20, WebGLVendor: "Google Inc. (NVIDIA)", WebGLRenderer: "ANGLE (NVIDIA, NVIDIA GeForce GTX 1080 Ti (0x00001B06) Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{Preset: "win-fhd125-irisxe", ScreenWidth: 1536, ScreenHeight: 864, AvailWidth: 1536, AvailHeight: 816, InnerWidth: 1520, InnerHeight: 721, OuterWidth: 1536, OuterHeight: 816, DevicePixelRatio: 1.25, DeviceMemory: 16, HardwareConcurrency: 12, WebGLVendor: "Google Inc. (Intel)", WebGLRenderer: "ANGLE (Intel, Intel(R) Iris(R) Xe Graphics (0x00009A49) Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{Preset: "win-hd-uhd620", ScreenWidth: 1366, ScreenHeight: 768, AvailWidth: 1366, AvailHeight: 720, InnerWidth: 1350, InnerHeight: 625, OuterWidth: 1366, OuterHeight: 720, DevicePixelRatio: 1, DeviceMemory: 8, HardwareConcurrency: 8, WebGLVendor: "Google Inc. (Intel)", WebGLRenderer: "ANGLE (Intel, Intel(R) UHD Graphics 620 (0x00005917) Direct3D11 vs_5_0 ps_5_0, D3D11)"},
	{Preset: "win-qhd125-rtx4070", ScreenWidth: 2560, ScreenHeight: 1440, AvailWidth: 2560, AvailHeight: 1392, InnerWidth: 2048, InnerHeight: 1118, OuterWidth: 2064, OuterHeight: 1213, DevicePixelRatio: 1.25, DeviceMemory: 32, HardwareConcurrency: 24, WebGLVendor: "Google Inc. (NVIDIA)", WebGLRenderer: "ANGLE (NVIDIA, NVIDIA GeForce RTX 4070 (0x00002782) Direct3D11 vs_5_0 ps_5_0, D3D11)"},
}

func newBrowserFingerprint() browserFingerprint {
	return newBrowserFingerprintWithReader(rand.Reader)
}

func newBrowserFingerprintWithReader(reader io.Reader) browserFingerprint {
	if reader == nil {
		reader = rand.Reader
	}
	index := 0
	if len(browserFingerprintPresets) > 1 {
		if value, err := rand.Int(reader, big.NewInt(int64(len(browserFingerprintPresets)))); err == nil {
			index = int(value.Int64())
		}
	}
	profile := browserFingerprintPresets[index]
	profile.ID = randomFingerprintID(reader)
	profile.CanvasSeed = randomUint32(reader, 0x6d2b79f5)
	profile.RandomSeed = randomUint32(reader, 0x9e3779b9)
	return profile
}

func randomFingerprintID(reader io.Reader) string {
	data := make([]byte, 16)
	if _, err := io.ReadFull(reader, data); err != nil {
		return "00000000-0000-4000-8000-000000000000"
	}
	data[6] = (data[6] & 0x0f) | 0x40
	data[8] = (data[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(data)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32]
}

func randomUint32(reader io.Reader, fallback uint32) uint32 {
	var data [4]byte
	if _, err := io.ReadFull(reader, data[:]); err != nil {
		return fallback
	}
	value := uint32(data[0])<<24 | uint32(data[1])<<16 | uint32(data[2])<<8 | uint32(data[3])
	if value == 0 {
		return fallback
	}
	return value
}

func (p browserFingerprint) jsProfile() jsScript.BrowserProfile {
	return jsScript.BrowserProfile{
		ID:                  p.ID,
		ScreenWidth:         p.ScreenWidth,
		ScreenHeight:        p.ScreenHeight,
		AvailWidth:          p.AvailWidth,
		AvailHeight:         p.AvailHeight,
		InnerWidth:          p.InnerWidth,
		InnerHeight:         p.InnerHeight,
		OuterWidth:          p.OuterWidth,
		OuterHeight:         p.OuterHeight,
		DevicePixelRatio:    p.DevicePixelRatio,
		DeviceMemory:        p.DeviceMemory,
		HardwareConcurrency: p.HardwareConcurrency,
		Platform:            "Win32",
		Language:            "zh-CN",
		Languages:           []string{"zh-CN", "zh"},
		TimezoneOffset:      -480,
		WebGLVendor:         p.WebGLVendor,
		WebGLRenderer:       p.WebGLRenderer,
		CanvasSeed:          p.CanvasSeed,
		RandomSeed:          p.RandomSeed,
	}
}

func (p browserFingerprint) screenSize() (int, int) {
	width, height := p.ScreenWidth, p.ScreenHeight
	if width <= 0 {
		width = defaultScreenWidth
	}
	if height <= 0 {
		height = defaultScreenHeight
	}
	return width, height
}
