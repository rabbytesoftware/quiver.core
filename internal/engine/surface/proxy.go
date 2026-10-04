package surface

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"
)

func unixTransport(socket string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", socket)
		},
		DisableCompression: true,
	}
}

// newProxy reverse-proxies to the unix socket. Streaming bodies are flushed
// immediately and WebSocket upgrades are handled by httputil.ReverseProxy.
// Credentials the daemon's own clients carry never reach the arrow.
func newProxy(socket string) http.Handler {
	return &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.Out.URL = &url.URL{Scheme: "http", Host: "localhost", Path: r.In.URL.Path, RawQuery: r.In.URL.RawQuery}
			r.Out.Host = "localhost"
			r.Out.Header.Del("Authorization")
			r.Out.Header.Del("Cookie")
		},
		Transport:     unixTransport(socket),
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, _ error) {
			http.Error(w, "arrow surface unavailable", http.StatusBadGateway)
		},
	}
}

// probe reports whether anything speaking HTTP answers on socket for path.
func probe(ctx context.Context, socket, path string) bool {
	client := &http.Client{
		Transport: unixTransport(socket),
		Timeout:   2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost"+path, nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return true
}
