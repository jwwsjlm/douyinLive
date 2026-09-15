package douyinLive

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
	"google.golang.org/protobuf/proto"
)

func TestProxyValidation(t *testing.T) {
	for _, raw := range []string{"", "http://localhost", "http://user@localhost:80/", "http://[::1]:8080", "socks5://u:p%40ss@localhost:1080"} {
		if err := ValidateProxyURL(raw); err != nil {
			t.Errorf("valid proxy rejected: %v", err)
		}
	}
	for _, raw := range []string{
		"localhost:8080", "https://host:443", "socks4://host:80", "socks5h://host:1080",
		"http://", "http://host:0", "http://host:65536", "http://host:abc", "http://host:",
		"http://host/path", "http://host?", "http://host#", "http://host?q=1",
		"http://host/#x", "http://user:secret@host:99999", "http://u:%0ap@host:80",
		"socks5://host", "http://::1:80", "http://host/\npath",
	} {
		if err := ValidateProxyURL(raw); err == nil || strings.Contains(err.Error(), "secret") {
			t.Errorf("invalid proxy accepted or credentials exposed: %v", err)
		}
	}
	if _, err := NewDouyinLiveWithOptions("room", nil, Options{ProxyURL: "http://u:secret@bad:0"}); err == nil {
		t.Fatal("constructor accepted invalid proxy")
	}
	if _, err := NewDouyinLiveWithOptions("room", nil, Options{SignProvider: SignProviderTikHub}); !errors.Is(err, ErrTikHubTokenEmpty) {
		t.Fatalf("missing TikHub token: %v", err)
	}
	if _, err := NewDouyinLiveWithOptions("room", nil, Options{SignProvider: "other"}); err == nil {
		t.Fatal("constructor accepted unknown signer")
	}
	var output bytes.Buffer
	dl, err := NewDouyinLiveWithOptions("room", log.New(&output, "", 0), Options{ProxyURL: "http://username:secret@localhost:80"})
	if err != nil {
		t.Fatal(err)
	}
	dl.Dispose()
	if strings.Contains(output.String(), "username") || strings.Contains(output.String(), "secret") || !strings.Contains(output.String(), "localhost:80") {
		t.Fatal("proxy log did not retain host or exposed credentials")
	}
}

func clearProxyEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy", "REQUEST_METHOD"} {
		t.Setenv(name, "")
	}
}

func TestProxyEnvironmentSnapshot(t *testing.T) {
	for _, lower := range []bool{false, true} {
		t.Run(fmt.Sprint(lower), func(t *testing.T) {
			clearProxyEnvironment(t)
			names := []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}
			if lower {
				for i := range names {
					names[i] = strings.ToLower(names[i])
				}
			}
			t.Setenv(names[0], "http://proxy-http:80")
			t.Setenv(names[1], "socks5://proxy-https:1080")
			t.Setenv(names[2], "bypass.test")
			policy, err := newProxyPolicy("")
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv(names[1], "http://changed:80")
			for target, want := range map[string]string{
				"http://remote.test": "proxy-http:80", "https://remote.test": "proxy-https:1080",
				"https://bypass.test": "", "https://127.0.0.1": "",
			} {
				r, _ := http.NewRequest(http.MethodGet, target, nil)
				u, err := policy.resolve(r)
				got := ""
				if u != nil {
					got = u.Host
				}
				if err != nil || got != want || !policy.disableHTTP3 {
					t.Fatalf("route %s = %s, %v", target, got, err)
				}
			}
			explicit, err := newProxyPolicy("http://explicit:80")
			if err != nil {
				t.Fatal(err)
			}
			r, _ := http.NewRequest(http.MethodGet, "https://bypass.test", nil)
			u, err := explicit.resolve(r)
			if err != nil || u.Host != "explicit:80" {
				t.Fatal("NO_PROXY overrode explicit proxy")
			}
		})
	}
	clearProxyEnvironment(t)
	policy, err := newProxyPolicy("")
	if err != nil || policy.disableHTTP3 {
		t.Fatal("direct policy disabled HTTP/3")
	}
	t.Setenv("HTTPS_PROXY", "https://u:secret@host:443")
	if _, err := newProxyPolicy(""); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("invalid environment proxy: %v", err)
	}
}

// testForwardProxy implements only the protocol messages exercised below.
// All destinations are forwarded to a local test server; it never dials the internet.
func testForwardProxy(t *testing.T, scheme, upstream, credentials string) (string, *atomic.Int64) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var count atomic.Int64
	var conns sync.Map
	var workers sync.WaitGroup
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			conns.Store(c, true)
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer conns.Delete(c)
				defer c.Close()
				_ = c.SetDeadline(time.Now().Add(10 * time.Second))
				reader := bufio.NewReader(c)
				var request *http.Request
				var err error
				if scheme == "http" {
					request, err = http.ReadRequest(reader)
					if err != nil {
						return
					}
					if credentials != "" && request.Header.Get("Proxy-Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte(credentials)) {
						_, _ = io.WriteString(c, "HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n")
						return
					}
				} else {
					header := make([]byte, 2)
					if _, err := io.ReadFull(reader, header); err != nil {
						return
					}
					if _, err := io.CopyN(io.Discard, reader, int64(header[1])); err != nil {
						return
					}
					method := byte(0)
					if credentials != "" {
						method = 2
					}
					_, _ = c.Write([]byte{5, method})
					if method == 2 {
						if _, err := io.ReadFull(reader, header); err != nil {
							return
						}
						user := make([]byte, int(header[1])+1)
						if _, err := io.ReadFull(reader, user); err != nil {
							return
						}
						password := make([]byte, int(user[len(user)-1]))
						if _, err := io.ReadFull(reader, password); err != nil {
							return
						}
						if string(user[:len(user)-1])+":"+string(password) != credentials {
							_, _ = c.Write([]byte{1, 1})
							return
						}
						_, _ = c.Write([]byte{1, 0})
					}
					connect := make([]byte, 5)
					if _, err := io.ReadFull(reader, connect); err != nil {
						return
					}
					if connect[3] != 3 {
						t.Error("SOCKS5 client did not delegate domain resolution")
						return
					}
					if _, err := io.CopyN(io.Discard, reader, int64(connect[4])+2); err != nil {
						return
					}
				}
				target, err := net.DialTimeout("tcp", upstream, time.Second)
				if err != nil {
					t.Error(err)
					return
				}
				defer target.Close()
				count.Add(1)
				if scheme == "socks5" {
					_, _ = c.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 80})
				} else if request.Method == http.MethodConnect {
					_, _ = io.WriteString(c, "HTTP/1.1 200 Connection Established\r\n\r\n")
				} else {
					request.Header.Del("Proxy-Authorization")
					if err := request.Write(target); err != nil {
						return
					}
				}
				done := make(chan struct{})
				go func() {
					_, _ = io.Copy(target, reader)
					_ = target.Close()
					close(done)
				}()
				_, _ = io.Copy(c, target)
				_ = c.Close()
				<-done
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-acceptDone
		conns.Range(func(key, _ any) bool { _ = key.(net.Conn).Close(); return true })
		workers.Wait()
	})
	u := &url.URL{Scheme: scheme, Host: listener.Addr().String()}
	if credentials != "" {
		user, password, _ := strings.Cut(credentials, ":")
		u.User = url.UserPassword(user, password)
	}
	return u.String(), &count
}

func TestProxyHTTPAndWebSocket(t *testing.T) {
	for _, scheme := range []string{"http", "socks5"} {
		for _, credentials := range []string{"", "user:password"} {
			t.Run(scheme+"/"+fmt.Sprint(credentials != ""), func(t *testing.T) {
				var requests, upgrades atomic.Int64
				upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Proxy-Authorization") != "" {
						t.Error("proxy credentials reached upstream")
					}
					if websocket.IsWebSocketUpgrade(r) {
						c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
						if err != nil {
							t.Error(err)
							return
						}
						upgrades.Add(1)
						defer c.Close()
						_ = c.WriteMessage(websocket.TextMessage, []byte("proxied"))
						return
					}
					requests.Add(1)
					_, _ = io.WriteString(w, "proxied")
				}))
				defer upstream.Close()
				proxyURL, count := testForwardProxy(t, scheme, upstream.Listener.Addr().String(), credentials)
				dl, err := NewDouyinLiveWithOptions("1001", log.New(io.Discard, "", 0), Options{ProxyURL: proxyURL})
				if err != nil {
					t.Fatal(err)
				}
				defer dl.Dispose()
				roots := x509.NewCertPool()
				roots.AddCert(upstream.Certificate())
				tlsConfig := &tls.Config{RootCAs: roots, ServerName: "example.com"}
				for round := 0; round < 2; round++ {
					if round > 0 {
						dl.rebuildHTTPClientAndHeaders()
					}
					dl.client.SetTLSClientConfig(tlsConfig.Clone())
					response, err := dl.client.R().Get("https://example.com/proxy-test")
					if err != nil {
						t.Fatal(err)
					}
					if body, err := response.ToString(); err != nil || body != "proxied" {
						t.Fatalf("HTTP body=%q err=%v", body, err)
					}
					dialer := *websocket.DefaultDialer
					dialer.TLSClientConfig = tlsConfig.Clone()
					c, wsResponse, err := dl.dialUpstreamWebSocket(&dialer, "wss://example.com/ws", nil, round+1)
					if err != nil {
						closeWebSocketHandshakeResponse(wsResponse)
						t.Fatal(err)
					}
					_, body, err := c.ReadMessage()
					_ = c.Close()
					if err != nil || string(body) != "proxied" {
						t.Fatalf("WS body=%q err=%v", body, err)
					}
				}
				if count.Load() != 4 || requests.Load() != 2 || upgrades.Load() != 2 {
					t.Fatalf("proxy=%d HTTP=%d WS=%d", count.Load(), requests.Load(), upgrades.Load())
				}
			})
		}
	}
}

func TestProxyPreparationAndReconnect(t *testing.T) {
	var paths sync.Map
	var upgrades atomic.Int64
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	upstream := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths.Store(r.URL.Path, true)
		w.Header().Set("Alt-Svc", fmt.Sprintf(`h3="%s"; ma=3600`, udp.LocalAddr()))
		switch r.URL.Path {
		case "/":
			http.SetCookie(w, &http.Cookie{Name: "ttwid", Value: "proxy-cookie"})
		case "/1001":
			_, _ = io.WriteString(w, `{"roomStore":{"roomInfo":{"room":{"id_str":"7659786040097426226","status":2,"title":"test","owner":{"id_str":"101220697463","nickname":"test"}}}}}`)
		case "/webcast/room/web/enter/":
			_, _ = io.WriteString(w, `{"status_code":0,"data":{"room":{"id_str":"7659786040097426226","status":2,"title":"test","owner":{"id_str":"101220697463","nickname":"test"}}}}`)
		case "/webcast/im/fetch/":
			body, _ := proto.Marshal(&new_douyin.Webcast_Im_Response{
				PushServer: "ws://live.douyin.com/webcast/im/push/v2/", Cursor: "proxy-cursor",
			})
			_, _ = w.Write(body)
		}
	}))
	defer upstream.Close()
	// Use a separate plaintext upstream for the internal WS dialer.
	ws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		upgrades.Add(1)
		defer c.Close()
		for {
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer ws.Close()
	proxyURL, _ := testForwardProxy(t, "http", upstream.Listener.Addr().String(), "")
	wsProxyURL, _ := testForwardProxy(t, "http", ws.Listener.Addr().String(), "")
	dl, err := newDouyinLiveWithProxy("1001", log.New(io.Discard, "", 0), "", staticWebsocketSigner{signature: "sig"}, proxyURL, "")
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Dispose()
	roots := x509.NewCertPool()
	roots.AddCert(upstream.Certificate())
	dl.client.SetTLSClientConfig(&tls.Config{RootCAs: roots, ServerName: "example.com"})
	// The same fixed policy routes HTTP/HTTPS to two local protocol endpoints.
	// Production policies use a single explicit proxy; routing is confined to this test.
	tlsProxy, _ := url.Parse(proxyURL)
	plainProxy, _ := url.Parse(wsProxyURL)
	dl.proxy.resolve = func(r *http.Request) (*url.URL, error) {
		if r.URL.Scheme == "http" {
			return plainProxy, nil
		}
		return tlsProxy, nil
	}
	if err := dl.startWebSocket(); err != nil {
		t.Fatal(err)
	}
	if !dl.reconnect(1, false, false) {
		t.Fatal("reconnect through proxy failed")
	}
	if status, err := dl.CheckLiveStatus(context.Background()); err != nil || status.Code != LiveStatusOnline {
		t.Fatalf("status through proxy: %+v %v", status, err)
	}
	for _, path := range []string{"/", "/1001", "/webcast/room/web/enter/", "/webcast/im/fetch/"} {
		if _, ok := paths.Load(path); !ok {
			t.Errorf("preparation did not request %s", path)
		}
	}
	if upgrades.Load() != 2 {
		t.Fatalf("WS connections = %d, want initial + reconnect", upgrades.Load())
	}
	_ = udp.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, _, err := udp.ReadFrom(make([]byte, 2048)); err == nil {
		t.Fatal("HTTP/3 bypassed proxy")
	}
}

func TestProxyFailureNeverDialsDirect(t *testing.T) {
	var direct atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		direct.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	for _, scheme := range []string{"http", "socks5"} {
		proxyURL, _ := testForwardProxy(t, scheme, upstream.Listener.Addr().String(), "user:password")
		u, _ := url.Parse(proxyURL)
		u.User = url.UserPassword("user", "wrong")
		dl, err := NewDouyinLiveWithOptions("1001", log.New(io.Discard, "", 0), Options{ProxyURL: u.String()})
		if err != nil {
			t.Fatal(err)
		}
		target := strings.Replace(upstream.URL, "127.0.0.1", "localhost", 1)
		response, err := dl.client.R().Get(target)
		if err == nil && response.GetStatusCode() < 400 {
			t.Fatal("proxy authentication unexpectedly succeeded")
		}
		c, responseWS, err := dl.dialUpstreamWebSocket(websocket.DefaultDialer, "ws"+strings.TrimPrefix(target, "http")+"/ws", nil, 1)
		closeWebSocketHandshakeResponse(responseWS)
		if c != nil || err == nil {
			t.Fatal("WS proxy authentication unexpectedly succeeded")
		}
		dl.Dispose()
	}
	if direct.Load() != 0 {
		t.Fatal("failed proxy request reached upstream")
	}
}

func TestProxyConcurrentListenersStayIsolated(t *testing.T) {
	var workers sync.WaitGroup
	for _, id := range []string{"1001", "1002"} {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 每个房间的 ttwid 必须是自己那份；其后允许跟上全局一致的 PC 客户端固定 Cookie。
			// Each room must carry its own ttwid; the fixed PC-client cookies may follow.
			cookie := r.Header.Get("Cookie")
			if !strings.HasPrefix(cookie, "ttwid="+id) {
				t.Errorf("room %s received another room's cookie: %q", id, cookie)
			}
			_, _ = io.WriteString(w, id)
		}))
		defer upstream.Close()
		proxyURL, _ := testForwardProxy(t, "http", upstream.Listener.Addr().String(), "")
		dl, err := NewDouyinLiveWithOptions(id, log.New(io.Discard, "", 0), Options{ProxyURL: proxyURL, Cookie: "ttwid=" + id})
		if err != nil {
			t.Fatal(err)
		}
		defer dl.Dispose()
		workers.Add(1)
		go func() {
			defer workers.Done()
			for attempt := 0; attempt < 3; attempt++ {
				response, err := dl.client.R().SetHeader("Cookie", dl.getCookieString()).Get("http://example.com/")
				if err != nil {
					t.Error(err)
					return
				}
				if body, err := response.ToString(); err != nil || body != id {
					t.Errorf("room %s reached wrong upstream: %q %v", id, body, err)
				}
			}
		}()
	}
	workers.Wait()
}

func TestProxySignerFallbackKeepsRoute(t *testing.T) {
	var attempts atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		c, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		_ = c.Close()
	}))
	defer upstream.Close()
	proxyURL, count := testForwardProxy(t, "http", upstream.Listener.Addr().String(), "")
	dl, err := NewDouyinLiveWithOptions("1001", log.New(io.Discard, "", 0), Options{ProxyURL: proxyURL})
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Dispose()
	dl.contextPrepared = true
	dl.updateRoomInfo("7659786040097426226", "101220697463", "", "", "")
	dl.wsPushURL = "ws://live.douyin.com/webcast/im/push/v2/"
	c, response, err := dl.dialWebSocketWithSignerFallback(websocket.DefaultDialer, dl.wsPushURL, nil, 1)
	if err != nil {
		closeWebSocketHandshakeResponse(response)
		t.Fatal(err)
	}
	_ = c.Close()
	if attempts.Load() != 2 || count.Load() != 2 || websocketSignerImplementationName(dl.signer) != "goja_fallback" {
		t.Fatal("signature fallback did not retry through the same proxy")
	}
}

func TestProxyCloseCancelsPendingHandshake(t *testing.T) {
	started := make(chan struct{})
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer c.Close()
		close(started)
		_, _ = io.Copy(io.Discard, c)
	}))
	defer proxy.Close()
	dl, err := NewDouyinLiveWithOptions("1001", log.New(io.Discard, "", 0), Options{ProxyURL: proxy.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Dispose()
	done := make(chan error, 1)
	go func() {
		_, response, err := dl.dialUpstreamWebSocket(websocket.DefaultDialer, "wss://example.com/ws", nil, 1)
		closeWebSocketHandshakeResponse(response)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("proxy handshake did not start")
	}
	dl.signalClose()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled handshake succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("closing listener did not cancel proxy handshake")
	}
}
