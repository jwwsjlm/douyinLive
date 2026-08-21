package jsScript

import (
	_ "embed"
	"encoding/json"
	"errors"
	"sync"

	"github.com/dop251/goja"
)

// 嵌入的 JavaScript 文件来源于开源项目，感谢贡献者们的努力。
// The embedded JavaScript file comes from an open-source project; thanks to its contributors.
//
//go:embed webmssdk.js
var jsScript string

var (
	programOnce sync.Once
	program     *goja.Program
	programErr  error

	// 以下变量仅用于兼容旧版 LoadGoja/ExecuteJS API。
	// 生产签名流程使用每个直播会话独立的 Signer，不再共享此 Runtime。
	legacyMu     sync.Mutex
	legacySigner *Signer
	vm           *goja.Runtime
)

// Signer 封装一个直播会话独享的 Goja Runtime。
// Signer owns one Goja runtime for a single live session.
//
// Goja Runtime 不支持并发调用，因此 Sign 会在实例内部串行执行。
// A Goja Runtime is not goroutine-safe, so Sign serializes calls per instance.
type Signer struct {
	mu        sync.Mutex
	vm        *goja.Runtime
	fGetSign  func(string) string
	userAgent string
	cookie    string
	profileID string
	closed    bool
}

// compiledProgram 返回全局预编译的 webmssdk.js 程序。
// compiledProgram returns the process-wide precompiled webmssdk.js program.
func compiledProgram() (*goja.Program, error) {
	programOnce.Do(func() {
		program, programErr = goja.Compile("webmssdk.js", jsScript, false)
	})
	return program, programErr
}

// NewSigner 创建一个使用指定 UA 和 Cookie 的独立签名运行时。
// NewSigner creates an isolated signing runtime for the supplied UA and cookie.
func NewSigner(ua, cookie string) (*Signer, error) {
	return NewSignerWithProfile(ua, cookie, DefaultBrowserProfile())
}

// NewSignerWithProfile 创建绑定指定 UA、Cookie 和浏览器画像的独立签名运行时。
// NewSignerWithProfile creates an isolated signer bound to the supplied UA, cookie, and browser profile.
func NewSignerWithProfile(ua, cookie string, profile BrowserProfile) (*Signer, error) {
	compiled, err := compiledProgram()
	if err != nil {
		return nil, err
	}

	profile = normalizeBrowserProfile(profile)
	runtime := goja.New()
	if _, err := runtime.RunString(browserEnvironmentScriptWithProfile(ua, cookie, profile)); err != nil {
		return nil, err
	}
	if _, err := runtime.RunProgram(compiled); err != nil {
		return nil, err
	}

	var getSign func(string) string
	if err := runtime.ExportTo(runtime.Get("get_sign"), &getSign); err != nil {
		return nil, err
	}
	if getSign == nil {
		return nil, errors.New("get_sign is not available")
	}

	return &Signer{
		vm:        runtime,
		fGetSign:  getSign,
		userAgent: ua,
		cookie:    cookie,
		profileID: profile.ID,
	}, nil
}

// Sign 反复调用当前会话 Runtime 中的 get_sign。
// Sign repeatedly invokes get_sign in this session's runtime.
func (s *Signer) Sign(signature string) (string, error) {
	if s == nil {
		return "", errors.New("signer is nil")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.vm == nil || s.fGetSign == nil {
		return "", errors.New("signer is closed")
	}
	return s.fGetSign(signature), nil
}

// ProfileMatches 判断 Runtime 是否与当前连接的 UA/Cookie 一致。
// ProfileMatches reports whether the runtime matches the current connection profile.
func (s *Signer) ProfileMatches(ua, cookie string) bool {
	return s.ProfileMatchesWithBrowserProfile(ua, cookie, "")
}

// ProfileMatchesWithBrowserProfile 判断 Runtime 是否同时匹配 UA、Cookie 和画像 ID。
// ProfileMatchesWithBrowserProfile reports whether the runtime matches the UA, cookie, and optional profile ID.
func (s *Signer) ProfileMatchesWithBrowserProfile(ua, cookie, profileID string) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.closed && s.vm != nil && s.userAgent == ua && s.cookie == cookie && (profileID == "" || s.profileID == profileID)
}

// Close 解除 Runtime 和 JS 函数引用，使其可由 Go GC 回收。
// Close drops runtime and function references so Go's GC can reclaim them.
func (s *Signer) Close() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.closed = true
	s.fGetSign = nil
	s.vm = nil
	s.userAgent = ""
	s.cookie = ""
	s.profileID = ""
	s.mu.Unlock()
}

// LoadGoja 将 JavaScript 加载到 Goja 运行时，并设置签名所需的浏览器环境。
// LoadGoja loads JavaScript into the Goja runtime and prepares the browser-like signing environment.
// 参数/Parameters:
//   - ua: 签名环境使用的浏览器 User-Agent。 Browser User-Agent used by the signing environment.
func LoadGoja(ua string) error {
	return LoadGojaWithCookie(ua, "")
}

// LoadGojaWithCookie replaces the legacy shared Goja signer with a runtime
// prepared from the supplied User-Agent and Cookie.
// LoadGojaWithCookie 使用指定的 User-Agent 和 Cookie 重建旧版共享 Goja 签名运行时。
func LoadGojaWithCookie(ua, cookie string) error {
	signer, err := NewSigner(ua, cookie)
	if err != nil {
		return err
	}

	legacyMu.Lock()
	oldSigner := legacySigner
	legacySigner = signer
	vm = signer.vm
	legacyMu.Unlock()
	if oldSigner != nil {
		oldSigner.Close()
	}
	return nil
}

// ExecuteJS 调用 JavaScript 中的 get_sign 函数生成签名。
// ExecuteJS calls the JavaScript get_sign function to generate a signature.
// 参数/Parameters:
//   - signature: 传入 get_sign 的 X-MS-STUB 字符串。 X-MS-STUB string passed to get_sign.
func ExecuteJS(signature string) string {
	legacyMu.Lock()
	defer legacyMu.Unlock()
	if legacySigner == nil {
		return ""
	}
	value, err := legacySigner.Sign(signature)
	if err != nil {
		return ""
	}
	return value
}

func browserEnvironmentScript(ua, cookie string) string {
	return browserEnvironmentScriptWithProfile(ua, cookie, DefaultBrowserProfile())
}

func browserEnvironmentScriptWithProfile(ua, cookie string, profile BrowserProfile) string {
	profile = normalizeBrowserProfile(profile)
	uaJSON, _ := json.Marshal(ua)
	cookieJSON, _ := json.Marshal(cookie)
	profileJSON, _ := json.Marshal(profile)
	return `
		(function () {
			var root = this;
			var ua = ` + string(uaJSON) + `;
			var cookie = ` + string(cookieJSON) + `;
			var profile = ` + string(profileJSON) + `;
			var webglDebugInfo = {
				UNMASKED_VENDOR_WEBGL: 37445,
				UNMASKED_RENDERER_WEBGL: 37446
			};
			var webglParameters = {};
			webglParameters[37445] = profile.webgl_vendor;
			webglParameters[37446] = profile.webgl_renderer;
			webglParameters[7938] = "WebGL 1.0 (OpenGL ES 2.0 Chromium)";
			webglParameters[35724] = "WebGL GLSL ES 1.0 (OpenGL ES GLSL ES 1.0 Chromium)";
			webglParameters[3379] = 16384;
			webglParameters[34076] = 16384;
			webglParameters[34024] = 16384;
			webglParameters[36347] = 4096;
			webglParameters[36348] = 30;
			webglParameters[36349] = 1024;
			webglParameters[34930] = 16;
			webglParameters[35660] = 16;
			webglParameters[3414] = 24;
			webglParameters[3415] = 24;
			webglParameters[3416] = 24;
			webglParameters[3410] = 8;
			webglParameters[3411] = 8;
			webglParameters[3412] = 8;
			webglParameters[3413] = 8;
			webglParameters[3418] = 24;
			webglParameters[3419] = 8;

			function makeStorage(seed) {
				var data = {};
				Object.keys(seed || {}).forEach(function (key) { data[key] = String(seed[key]); });
				return {
					get length() { return Object.keys(data).length; },
					key: function (index) { return Object.keys(data)[index] || null; },
					getItem: function (key) {
						key = String(key);
						return Object.prototype.hasOwnProperty.call(data, key) ? data[key] : null;
					},
					setItem: function (key, value) { data[String(key)] = String(value); },
					removeItem: function (key) { delete data[String(key)]; },
					clear: function () { data = {}; }
				};
			}

			function makeThenable(value) {
				return {
					then: function (resolve) {
						if (typeof resolve === "function") {
							resolve(value);
						}
						return makeThenable(value);
					},
					catch: function () { return makeThenable(value); }
				};
			}

			function makeCookieJar(header) {
				var jar = {};
				String(header || "").split(";").forEach(function (part) {
					var trimmed = part.trim();
					if (!trimmed) {
						return;
					}
					var index = trimmed.indexOf("=");
					if (index <= 0) {
						return;
					}
					jar[trimmed.slice(0, index).trim()] = trimmed.slice(index + 1).trim();
				});
				return {
					get: function () {
						return Object.keys(jar).map(function (name) {
							return name + "=" + jar[name];
						}).join("; ");
					},
					set: function (value) {
						var parts = String(value || "").split(";");
						var pair = (parts.shift() || "").trim();
						var index = pair.indexOf("=");
						if (index <= 0) {
							return;
						}
						var name = pair.slice(0, index).trim();
						var cookieValue = pair.slice(index + 1).trim();
						var remove = false;
						parts.forEach(function (attr) {
							var pieces = attr.trim().split("=");
							var key = String(pieces[0] || "").toLowerCase();
							var attrValue = pieces.slice(1).join("=");
							if (key === "max-age" && Number(attrValue) <= 0) {
								remove = true;
							}
							if (key === "expires") {
								var expiresAt = Date.parse(attrValue);
								if (!isNaN(expiresAt) && expiresAt <= Date.now()) {
									remove = true;
								}
							}
						});
						if (remove) {
							delete jar[name];
							return;
						}
						jar[name] = cookieValue;
					}
				};
			}

			function make2DContext() {
				return {
					fillStyle: "#000000",
					font: "10px sans-serif",
					textBaseline: "alphabetic",
					fillRect: function () {},
					clearRect: function () {},
					fillText: function () {},
					measureText: function (text) { return { width: String(text || "").length * (6 + (profile.canvas_seed % 7) / 100) }; },
					getImageData: function () { return { data: [profile.canvas_seed & 3, (profile.canvas_seed >>> 2) & 3, (profile.canvas_seed >>> 4) & 3, 255] }; },
					putImageData: function () {},
					beginPath: function () {},
					closePath: function () {},
					stroke: function () {},
					arc: function () {},
					save: function () {},
					restore: function () {}
				};
			}

			function makeWebGLContext() {
				return {
					VERSION: 7938,
					SHADING_LANGUAGE_VERSION: 35724,
					MAX_TEXTURE_SIZE: 3379,
					MAX_CUBE_MAP_TEXTURE_SIZE: 34076,
					MAX_RENDERBUFFER_SIZE: 34024,
					MAX_VERTEX_UNIFORM_VECTORS: 36347,
					MAX_VARYING_VECTORS: 36348,
					MAX_FRAGMENT_UNIFORM_VECTORS: 36349,
					MAX_TEXTURE_IMAGE_UNITS: 34930,
					MAX_VERTEX_TEXTURE_IMAGE_UNITS: 35660,
					RED_BITS: 3410,
					GREEN_BITS: 3411,
					BLUE_BITS: 3412,
					ALPHA_BITS: 3413,
					DEPTH_BITS: 3414,
					STENCIL_BITS: 3415,
					getExtension: function (name) {
						if (name === "WEBGL_debug_renderer_info") {
							return webglDebugInfo;
						}
						if (name === "EXT_texture_filter_anisotropic" ||
							name === "WEBKIT_EXT_texture_filter_anisotropic" ||
							name === "MOZ_EXT_texture_filter_anisotropic") {
							return { MAX_TEXTURE_MAX_ANISOTROPY_EXT: 34047 };
						}
						return null;
					},
					getParameter: function (param) {
						return Object.prototype.hasOwnProperty.call(webglParameters, param) ? webglParameters[param] : 0;
					},
					getSupportedExtensions: function () {
						return ["WEBGL_debug_renderer_info", "EXT_texture_filter_anisotropic"];
					},
					getContextAttributes: function () {
						return { alpha: true, antialias: true, depth: true, stencil: false };
					}
				};
			}

			function makeElement(tagName) {
				tagName = String(tagName || "").toLowerCase();
				var element = {
					tagName: tagName.toUpperCase(),
					style: {},
					children: [],
					width: 300,
					height: 150,
					appendChild: function (child) { this.children.push(child); return child; },
					removeChild: function (child) {
						var index = this.children.indexOf(child);
						if (index >= 0) {
							this.children.splice(index, 1);
						}
						return child;
					},
					setAttribute: function (key, value) { this[key] = String(value); },
					getAttribute: function (key) { return this[key] || null; },
					addEventListener: function () {},
					removeEventListener: function () {},
					getBoundingClientRect: function () {
						return { left: 0, top: 0, width: this.width || 0, height: this.height || 0 };
					}
				};
				if (tagName === "canvas") {
					element.toDataURL = function () { return "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB" + profile.canvas_seed.toString(16); };
					element.getContext = function (type) {
						if (type === "2d") {
							return make2DContext();
						}
						if (type === "webgl" || type === "experimental-webgl") {
							return makeWebGLContext();
						}
						return null;
					};
				}
				return element;
			}

			root.window = root;
			root.self = root;
			root.top = root;
			root.parent = root;
			root.location = root.location || {
				href: "https://live.douyin.com/",
				origin: "https://live.douyin.com",
				protocol: "https:",
				host: "live.douyin.com",
				hostname: "live.douyin.com",
				pathname: "/",
				search: "",
				hash: ""
			};
			root.screen = {
				width: profile.screen_width,
				height: profile.screen_height,
				availWidth: profile.avail_width,
				availHeight: profile.avail_height,
				colorDepth: 24,
				pixelDepth: 24
			};
			root.devicePixelRatio = profile.device_pixel_ratio;
			root.innerWidth = profile.inner_width;
			root.innerHeight = profile.inner_height;
			root.outerWidth = profile.outer_width;
			root.outerHeight = profile.outer_height;
			root.screenX = 0;
			root.screenY = 0;
			root.pageXOffset = 0;
			root.pageYOffset = 0;

			root.localStorage = makeStorage({
				"__msuuid__": profile.id,
				"xmst": "",
				"websocketkey20230220": "",
				"a11y_device_id": profile.id,
				"RTC_DEVICE_ID": profile.id
			});
			root.sessionStorage = makeStorage({
				"__tea_session_id_6383": "",
				"sessionStarted": "1"
			});
			root.indexedDB = {};

			root.navigator = {
				userAgent: ua,
				appCodeName: "Mozilla",
				appName: "Netscape",
				appVersion: ua.replace(/^Mozilla\//, ""),
				platform: profile.platform,
				product: "Gecko",
				productSub: "20030107",
				vendor: "Google Inc.",
				vendorSub: "",
				language: profile.language,
				languages: profile.languages,
				cookieEnabled: true,
				onLine: true,
				doNotTrack: null,
				deviceMemory: profile.device_memory,
				hardwareConcurrency: profile.hardware_concurrency,
				maxTouchPoints: 0,
				webdriver: false,
				plugins: [
					{ name: "PDF Viewer", filename: "internal-pdf-viewer", description: "Portable Document Format" },
					{ name: "Chrome PDF Viewer", filename: "internal-pdf-viewer", description: "Portable Document Format" },
					{ name: "Chromium PDF Viewer", filename: "internal-pdf-viewer", description: "Portable Document Format" },
					{ name: "Microsoft Edge PDF Viewer", filename: "internal-pdf-viewer", description: "Portable Document Format" },
					{ name: "WebKit built-in PDF", filename: "internal-pdf-viewer", description: "Portable Document Format" }
				],
				mimeTypes: [{ type: "application/pdf" }],
				sendBeacon: function () { return true; },
				vibrate: function () { return true; },
				getBattery: function () {
					return makeThenable({
						charging: true,
						chargingTime: 0,
						dischargingTime: Infinity,
						level: 1
					});
				}
			};

			var cookieJar = makeCookieJar(cookie);
			root.document = {
				referrer: "https://www.douyin.com/",
				visibilityState: "visible",
				hidden: false,
				compatMode: "CSS1Compat",
				readyState: "complete",
				documentElement: makeElement("html"),
				head: makeElement("head"),
				body: makeElement("body"),
				createElement: makeElement,
				createEvent: function () { return { initEvent: function () {} }; },
				addEventListener: function () {},
				removeEventListener: function () {},
				getElementsByTagName: function () { return []; }
			};
			Object.defineProperty(root.document, "cookie", {
				get: function () { return cookieJar.get(); },
				set: function (value) { cookieJar.set(value); }
			});

			root.Image = function () { return makeElement("img"); };
			root.TouchEvent = function () {};
			root.RTCPeerConnection = function () {
				return {
					createDataChannel: function () { return {}; },
					createOffer: function () { return makeThenable({ sdp: "" }); },
					setLocalDescription: function () { return makeThenable(undefined); },
					close: function () {},
					addEventListener: function () {},
					removeEventListener: function () {}
				};
			};
			root.webkitRTCPeerConnection = root.RTCPeerConnection;
			root.mozRTCPeerConnection = root.RTCPeerConnection;

			root.crypto = root.crypto || {};
			var cryptoState = profile.random_seed >>> 0;
			root.crypto.getRandomValues = root.crypto.getRandomValues || function (array) {
				for (var i = 0; i < array.length; i++) {
					cryptoState ^= cryptoState << 13;
					cryptoState ^= cryptoState >>> 17;
					cryptoState ^= cryptoState << 5;
					array[i] = cryptoState & 255;
				}
				return array;
			};
			root.addEventListener = function () {};
			root.removeEventListener = function () {};
			root.setTimeout = function (fn) {
				if (typeof fn === "function") {
					fn();
				}
				return 1;
			};
			root.clearTimeout = function () {};
			root.setInterval = function () { return 1; };
			root.clearInterval = function () {};
			root.Date.prototype.getTimezoneOffset = function () { return profile.timezone_offset; };
		}).call(this);
	`
}
