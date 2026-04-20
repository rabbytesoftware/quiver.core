//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type client struct {
	baseURL string
	http    *http.Client
}

func newClient(baseURL string) *client {
	return &client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *client) Add(ns string) *http.Response {
	req, _ := http.NewRequest(http.MethodPost, c.url("/v0/arrow/"+url.PathEscape(ns)), nil)
	resp, _ := c.http.Do(req)
	return resp
}

func (c *client) Remove(ns string) *http.Response {
	req, _ := http.NewRequest(http.MethodDelete, c.url("/v0/arrow/"+url.PathEscape(ns)), nil)
	resp, _ := c.http.Do(req)
	return resp
}

func (c *client) List() *http.Response {
	resp, _ := c.http.Get(c.url("/v0/arrow"))
	return resp
}

func (c *client) GetDetail(ns string) *http.Response {
	resp, _ := c.http.Get(c.url("/v0/arrow/" + url.PathEscape(ns)))
	return resp
}

func (c *client) Update(ns string, body map[string]any) *http.Response {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPatch, c.url("/v0/arrow/"+url.PathEscape(ns)), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := c.http.Do(req)
	return resp
}

func (c *client) Install(ns string, vars map[string]string) *http.Response {
	return c.Execute(ns, "_install", vars)
}

func (c *client) Uninstall(ns string, vars map[string]string) *http.Response {
	return c.Execute(ns, "_uninstall", vars)
}

func (c *client) Execute(ns, method string, vars map[string]string) *http.Response {
	body := map[string]any{}
	if len(vars) > 0 {
		body["variables"] = vars
	}
	b, _ := json.Marshal(body)
	path := "/v0/arrow/" + url.PathEscape(ns) + "/" + url.PathEscape(method)
	req, _ := http.NewRequest(http.MethodPost, c.url(path), bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := c.http.Do(req)
	return resp
}

func (c *client) Stop(ns string) *http.Response {
	return c.Execute(ns, "_stop", nil)
}

func (c *client) Seed(ns string, body []byte) *http.Response {
	req, _ := http.NewRequest("SEED", c.url("/v0/arrow/"+url.PathEscape(ns)), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/yaml")
	resp, _ := c.http.Do(req)
	return resp
}

func (c *client) Validate(ns string, body []byte) *http.Response {
	req, _ := http.NewRequest("SEED", c.url("/v0/arrow/"+url.PathEscape(ns)+"/validate"), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/yaml")
	resp, _ := c.http.Do(req)
	return resp
}

func (c *client) DialRuntime(ns string) (*websocket.Conn, *http.Response, error) {
	wsURL := strings.Replace(c.baseURL, "http://", "ws://", 1)
	wsURL += "/v0/arrow.runtime/" + url.PathEscape(ns)
	return websocket.DefaultDialer.Dial(wsURL, nil)
}

func (c *client) url(path string) string {
	return c.baseURL + path
}

// mustStatus asserts resp.StatusCode == want, logs body on failure, closes body.
func mustStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status %d, got %d\nbody: %s", want, resp.StatusCode, body)
	}
}

// decodeJSON decodes resp.Body into v and closes the body.
func decodeJSON(t *testing.T, resp *http.Response, v any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decodeJSON: %v", err)
	}
}

func (e *Env) client() *client {
	return newClient(e.URL)
}
