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
	t       *testing.T
	baseURL string
	http    *http.Client
}

func newClient(t *testing.T, baseURL string) *client {
	t.Helper()
	return &client{
		t:       t,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *client) Add(ns string) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.url("/v0/arrow/"+url.PathEscape(ns)), nil)
	if err != nil {
		c.t.Fatalf("Add: create request: %v", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Add: do request: %v", err)
	}
	return resp
}

func (c *client) Remove(ns string) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodDelete, c.url("/v0/arrow/"+url.PathEscape(ns)), nil)
	if err != nil {
		c.t.Fatalf("Remove: create request: %v", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Remove: do request: %v", err)
	}
	return resp
}

func (c *client) List() *http.Response {
	c.t.Helper()
	resp, err := c.http.Get(c.url("/v0/arrow"))
	if err != nil {
		c.t.Fatalf("List: do request: %v", err)
	}
	return resp
}

func (c *client) GetDetail(ns string) *http.Response {
	c.t.Helper()
	resp, err := c.http.Get(c.url("/v0/arrow/" + url.PathEscape(ns)))
	if err != nil {
		c.t.Fatalf("GetDetail: do request: %v", err)
	}
	return resp
}

func (c *client) Update(ns string, body map[string]any) *http.Response {
	c.t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		c.t.Fatalf("Update: marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPatch, c.url("/v0/arrow/"+url.PathEscape(ns)), bytes.NewReader(b))
	if err != nil {
		c.t.Fatalf("Update: create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Update: do request: %v", err)
	}
	return resp
}

func (c *client) Install(ns string, vars map[string]string) *http.Response {
	return c.Execute(ns, "_install", vars)
}

func (c *client) Uninstall(ns string, vars map[string]string) *http.Response {
	return c.Execute(ns, "_uninstall", vars)
}

func (c *client) Execute(ns, method string, vars map[string]string) *http.Response {
	c.t.Helper()
	body := map[string]any{}
	if len(vars) > 0 {
		body["variables"] = vars
	}
	b, err := json.Marshal(body)
	if err != nil {
		c.t.Fatalf("Execute: marshal body: %v", err)
	}
	path := "/v0/arrow/" + url.PathEscape(ns) + "/" + url.PathEscape(method)
	req, err := http.NewRequest(http.MethodPost, c.url(path), bytes.NewReader(b))
	if err != nil {
		c.t.Fatalf("Execute: create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Execute: do request: %v", err)
	}
	return resp
}

func (c *client) Stop(ns string) *http.Response {
	return c.Execute(ns, "_stop", nil)
}

func (c *client) Seed(ns string, body []byte) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest("SEED", c.url("/v0/arrow/"+url.PathEscape(ns)), bytes.NewReader(body))
	if err != nil {
		c.t.Fatalf("Seed: create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/yaml")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Seed: do request: %v", err)
	}
	return resp
}

func (c *client) Validate(ns string, body []byte) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest("SEED", c.url("/v0/arrow/"+url.PathEscape(ns)+"/validate"), bytes.NewReader(body))
	if err != nil {
		c.t.Fatalf("Validate: create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/yaml")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Validate: do request: %v", err)
	}
	return resp
}

func (c *client) DialRuntime(ns string) (*websocket.Conn, error) {
	c.t.Helper()
	wsURL := strings.Replace(c.baseURL, "http://", "ws://", 1)
	wsURL += "/v0/arrow.runtime/" + url.PathEscape(ns)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	return conn, err
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

func (e *Env) client(t *testing.T) *client {
	return newClient(t, e.URL)
}
