package jsScript

import (
	_ "embed"
	"encoding/json"
	"sync"

	"github.com/dop251/goja"
)

// 嵌入的 JavaScript 文件来源于开源项目，感谢贡献者们的努力。
// The embedded JavaScript file comes from an open-source project; thanks to its contributors.
//
//go:embed webmssdk.js
var jsScript string

var (
	vm       *goja.Runtime
	fGetSign func(string) string
	mu       sync.Mutex
)

// LoadGoja 将 JavaScript 加载到 Goja 运行时，并设置签名所需的浏览器环境。
// LoadGoja loads JavaScript into the Goja runtime and prepares the browser-like signing environment.
func LoadGoja(ua string) error {
	return LoadGojaWithCookie(ua, "")
}

func LoadGojaWithCookie(ua, cookie string) error {
	mu.Lock()
	defer mu.Unlock()
	vm = goja.New()

	if _, err := vm.RunString(browserEnvironmentScript(ua, cookie) + jsScript); err != nil {
		return err
	}

	return vm.ExportTo(vm.Get("get_sign"), &fGetSign)
}

// ExecuteJS 调用 JavaScript 中的 get_sign 函数生成签名。
// ExecuteJS calls the JavaScript get_sign function to generate a signature.
func ExecuteJS(signature string) string {
	mu.Lock()
	defer mu.Unlock()
	return fGetSign(signature)
}

func browserEnvironmentScript(ua, cookie string) string {
	uaJSON, _ := json.Marshal(ua)
	cookieJSON, _ := json.Marshal(cookie)
	return `
		(function () {
			var root = this;
			var ua = ` + string(uaJSON) + `;
			var cookie = ` + string(cookieJSON) + `;
			var webglDebugInfo = {
				UNMASKED_VENDOR_WEBGL: 37445,
				UNMASKED_RENDERER_WEBGL: 37446
			};
			var webglParameters = {};
			webglParameters[37445] = "Google Inc. (NVIDIA)";
			webglParameters[37446] = "ANGLE (NVIDIA, NVIDIA GeForce GTX 1080 Ti (0x00001B06) Direct3D11 vs_5_0 ps_5_0, D3D11)";
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
					measureText: function (text) { return { width: String(text || "").length * 6 }; },
					getImageData: function () { return { data: [0, 0, 0, 255] }; },
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
					element.toDataURL = function () { return "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAAB"; };
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
				width: 1920,
				height: 1080,
				availWidth: 1920,
				availHeight: 1032,
				colorDepth: 24,
				pixelDepth: 24
			};
			root.devicePixelRatio = 1;
			root.innerWidth = 929;
			root.innerHeight = 917;
			root.outerWidth = 945;
			root.outerHeight = 1012;
			root.screenX = 0;
			root.screenY = 0;
			root.pageXOffset = 0;
			root.pageYOffset = 0;

			root.localStorage = makeStorage({
				"__msuuid__": "00000000-0000-4000-8000-000000000000",
				"xmst": "",
				"websocketkey20230220": "",
				"a11y_device_id": "",
				"RTC_DEVICE_ID": ""
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
				platform: "Win32",
				product: "Gecko",
				productSub: "20030107",
				vendor: "Google Inc.",
				vendorSub: "",
				language: "zh-CN",
				languages: ["zh-CN", "zh"],
				cookieEnabled: true,
				onLine: true,
				doNotTrack: null,
				deviceMemory: 32,
				hardwareConcurrency: 20,
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
			root.crypto.getRandomValues = root.crypto.getRandomValues || function (array) {
				for (var i = 0; i < array.length; i++) {
					array[i] = (i * 17 + 29) & 255;
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
			root.Date.prototype.getTimezoneOffset = function () { return -480; };
		}).call(this);
	`
}
