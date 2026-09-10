package douyinLive

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/net/http/httpproxy"
)

// ValidateProxyURL validates an optional HTTP CONNECT or SOCKS5 proxy URL.
// Errors deliberately omit the input, which may contain credentials.
func ValidateProxyURL(raw string) error {
	_, err := parseProxyURL(raw)
	return err
}

func parseProxyURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	invalid := errors.New("代理 URL 无效：请使用 http://host:port 或 socks5://host:port")
	if strings.ContainsFunc(raw, unicode.IsControl) {
		return nil, invalid
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, invalid
	}
	u.Scheme = strings.ToLower(u.Scheme)
	if u.Scheme != "http" && u.Scheme != "socks5" {
		return nil, errors.New("代理协议仅支持 http 和 socks5")
	}
	if u.Hostname() == "" || u.Opaque != "" || (u.Path != "" && u.Path != "/") ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") ||
		strings.ContainsAny(u.Host, "\\ \t\r\n") || strings.HasSuffix(u.Host, ":") {
		return nil, invalid
	}
	if strings.Contains(u.Hostname(), ":") && (!strings.HasPrefix(u.Host, "[") || net.ParseIP(u.Hostname()) == nil) {
		return nil, invalid
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return nil, invalid
		}
	} else if u.Scheme == "socks5" {
		return nil, errors.New("SOCKS5 代理必须指定端口")
	}
	if u.User != nil {
		password, _ := u.User.Password()
		if strings.ContainsFunc(u.User.Username()+password, unicode.IsControl) {
			return nil, invalid
		}
		// Gorilla only sends HTTP basic authentication when a password is present.
		u.User = url.UserPassword(u.User.Username(), password)
	}
	u.Path = ""
	return u, nil
}

// proxyPolicy is immutable for the lifetime of a listener, including reconnects.
type proxyPolicy struct {
	resolve      func(*http.Request) (*url.URL, error)
	disableHTTP3 bool
}

func newProxyPolicy(raw string) (proxyPolicy, error) {
	u, err := parseProxyURL(raw)
	if err != nil {
		return proxyPolicy{}, err
	}
	if u != nil {
		return proxyPolicy{resolve: http.ProxyURL(u), disableHTTP3: true}, nil
	}
	// Snapshot Go's environment proxy rules once so HTTP rebuilds and WS agree.
	env := httpproxy.FromEnvironment()
	for _, value := range []*string{&env.HTTPProxy, &env.HTTPSProxy} {
		if *value == "" {
			continue
		}
		raw := *value
		if !strings.Contains(raw, "://") {
			raw = "http://" + raw
		}
		u, err := parseProxyURL(raw)
		if err != nil || u == nil {
			return proxyPolicy{}, errors.New("环境变量代理无效：仅支持 HTTP CONNECT 和 SOCKS5")
		}
		*value = u.String()
	}
	resolve := env.ProxyFunc()
	return proxyPolicy{
		resolve: func(r *http.Request) (*url.URL, error) {
			return resolve(r.URL)
		},
		disableHTTP3: env.HTTPProxy != "" || env.HTTPSProxy != "",
	}, nil
}
