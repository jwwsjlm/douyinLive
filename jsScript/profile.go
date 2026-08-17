package jsScript

// BrowserProfile 描述 webmssdk.js 运行时使用的一组相互一致的浏览器环境参数。
// BrowserProfile describes one coherent browser environment used by webmssdk.js.
type BrowserProfile struct {
	ID                  string   `json:"id"`
	ScreenWidth         int      `json:"screen_width"`
	ScreenHeight        int      `json:"screen_height"`
	AvailWidth          int      `json:"avail_width"`
	AvailHeight         int      `json:"avail_height"`
	InnerWidth          int      `json:"inner_width"`
	InnerHeight         int      `json:"inner_height"`
	OuterWidth          int      `json:"outer_width"`
	OuterHeight         int      `json:"outer_height"`
	DevicePixelRatio    float64  `json:"device_pixel_ratio"`
	DeviceMemory        int      `json:"device_memory"`
	HardwareConcurrency int      `json:"hardware_concurrency"`
	Platform            string   `json:"platform"`
	Language            string   `json:"language"`
	Languages           []string `json:"languages"`
	TimezoneOffset      int      `json:"timezone_offset"`
	WebGLVendor         string   `json:"webgl_vendor"`
	WebGLRenderer       string   `json:"webgl_renderer"`
	CanvasSeed          uint32   `json:"canvas_seed"`
	RandomSeed          uint32   `json:"random_seed"`
}

// DefaultBrowserProfile 返回与旧版环境兼容的默认画像。
// DefaultBrowserProfile returns the legacy-compatible default environment profile.
func DefaultBrowserProfile() BrowserProfile {
	return BrowserProfile{
		ID:                  "00000000-0000-4000-8000-000000000000",
		ScreenWidth:         1920,
		ScreenHeight:        1080,
		AvailWidth:          1920,
		AvailHeight:         1032,
		InnerWidth:          929,
		InnerHeight:         917,
		OuterWidth:          945,
		OuterHeight:         1012,
		DevicePixelRatio:    1,
		DeviceMemory:        32,
		HardwareConcurrency: 20,
		Platform:            "Win32",
		Language:            "zh-CN",
		Languages:           []string{"zh-CN", "zh"},
		TimezoneOffset:      -480,
		WebGLVendor:         "Google Inc. (NVIDIA)",
		WebGLRenderer:       "ANGLE (NVIDIA, NVIDIA GeForce GTX 1080 Ti (0x00001B06) Direct3D11 vs_5_0 ps_5_0, D3D11)",
		CanvasSeed:          29,
		RandomSeed:          0x6d2b79f5,
	}
}

func normalizeBrowserProfile(profile BrowserProfile) BrowserProfile {
	defaults := DefaultBrowserProfile()
	if profile.ID == "" {
		profile.ID = defaults.ID
	}
	if profile.ScreenWidth <= 0 {
		profile.ScreenWidth = defaults.ScreenWidth
	}
	if profile.ScreenHeight <= 0 {
		profile.ScreenHeight = defaults.ScreenHeight
	}
	if profile.AvailWidth <= 0 {
		profile.AvailWidth = profile.ScreenWidth
	}
	if profile.AvailHeight <= 0 || profile.AvailHeight > profile.ScreenHeight {
		profile.AvailHeight = profile.ScreenHeight - 48
	}
	if profile.OuterWidth <= 0 || profile.OuterWidth > profile.ScreenWidth {
		profile.OuterWidth = profile.ScreenWidth
	}
	if profile.OuterHeight <= 0 || profile.OuterHeight > profile.AvailHeight {
		profile.OuterHeight = profile.AvailHeight
	}
	if profile.InnerWidth <= 0 || profile.InnerWidth >= profile.OuterWidth {
		profile.InnerWidth = max(1, profile.OuterWidth-16)
	}
	if profile.InnerHeight <= 0 || profile.InnerHeight >= profile.OuterHeight {
		profile.InnerHeight = max(1, profile.OuterHeight-95)
	}
	if profile.DevicePixelRatio <= 0 {
		profile.DevicePixelRatio = defaults.DevicePixelRatio
	}
	if profile.DeviceMemory <= 0 {
		profile.DeviceMemory = defaults.DeviceMemory
	}
	if profile.HardwareConcurrency <= 0 {
		profile.HardwareConcurrency = defaults.HardwareConcurrency
	}
	if profile.Platform == "" {
		profile.Platform = defaults.Platform
	}
	if profile.Language == "" {
		profile.Language = defaults.Language
	}
	if len(profile.Languages) == 0 {
		profile.Languages = append([]string(nil), defaults.Languages...)
	}
	if profile.WebGLVendor == "" {
		profile.WebGLVendor = defaults.WebGLVendor
	}
	if profile.WebGLRenderer == "" {
		profile.WebGLRenderer = defaults.WebGLRenderer
	}
	if profile.RandomSeed == 0 {
		profile.RandomSeed = defaults.RandomSeed
	}
	return profile
}
