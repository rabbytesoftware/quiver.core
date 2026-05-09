//go:build integration

package kit

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

// Client is the raw HTTP test client.
// Use it directly only for error-path tests that need to inspect raw status codes
// without decoding a response body. For happy-path tests, use TypedClient.
type Client struct {
	t       *testing.T
	baseURL string
	http    *http.Client
}

// NewClient creates a raw Client pointed at baseURL.
func NewClient(t *testing.T, baseURL string) *Client {
	t.Helper()
	return &Client{
		t:       t,
		baseURL: baseURL,
		http:    &http.Client{Timeout: 120 * time.Second},
	}
}

func (c *Client) Add(ns string) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.url("/v0/arrow/"+url.PathEscape(ns)), nil)
	if err != nil {
		c.t.Fatalf("Client.Add: create request: %v", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Client.Add: do request: %v", err)
	}
	return resp
}

func (c *Client) Remove(ns string) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodDelete, c.url("/v0/arrow/"+url.PathEscape(ns)), nil)
	if err != nil {
		c.t.Fatalf("Client.Remove: create request: %v", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Client.Remove: do request: %v", err)
	}
	return resp
}

func (c *Client) List() *http.Response {
	c.t.Helper()
	resp, err := c.http.Get(c.url("/v0/arrow"))
	if err != nil {
		c.t.Fatalf("Client.List: do request: %v", err)
	}
	return resp
}

func (c *Client) GetDetail(ns string) *http.Response {
	c.t.Helper()
	resp, err := c.http.Get(c.url("/v0/arrow/" + url.PathEscape(ns)))
	if err != nil {
		c.t.Fatalf("Client.GetDetail: do request: %v", err)
	}
	return resp
}

func (c *Client) Update(ns string, body map[string]any) *http.Response {
	c.t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		c.t.Fatalf("Client.Update: marshal body: %v", err)
	}
	req, err := http.NewRequest(http.MethodPatch, c.url("/v0/arrow/"+url.PathEscape(ns)), bytes.NewReader(b))
	if err != nil {
		c.t.Fatalf("Client.Update: create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Client.Update: do request: %v", err)
	}
	return resp
}

func (c *Client) Execute(ns, method string, vars map[string]string) *http.Response {
	c.t.Helper()
	body := map[string]any{}
	if len(vars) > 0 {
		body["variables"] = vars
	}
	b, err := json.Marshal(body)
	if err != nil {
		c.t.Fatalf("Client.Execute: marshal body: %v", err)
	}
	path := "/v0/runtime/" + url.PathEscape(ns) + "/" + url.PathEscape(method)
	req, err := http.NewRequest(http.MethodPost, c.url(path), bytes.NewReader(b))
	if err != nil {
		c.t.Fatalf("Client.Execute: create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Client.Execute: do request: %v", err)
	}
	return resp
}

func (c *Client) Install(ns string, vars map[string]string) *http.Response {
	return c.Execute(ns, "install", vars)
}

func (c *Client) Uninstall(ns string, vars map[string]string) *http.Response {
	return c.Execute(ns, "uninstall", vars)
}

func (c *Client) Stop(ns string) *http.Response {
	return c.Execute(ns, "stop", nil)
}

func (c *Client) Seed(ns string, body []byte) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.url("/v0/arrow/"+url.PathEscape(ns)+"/manifest"), bytes.NewReader(body))
	if err != nil {
		c.t.Fatalf("Client.Seed: create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/yaml")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Client.Seed: do request: %v", err)
	}
	return resp
}

func (c *Client) Validate(ns string, body []byte) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.url("/v0/arrow/"+url.PathEscape(ns)+"/manifest/validate"), bytes.NewReader(body))
	if err != nil {
		c.t.Fatalf("Client.Validate: create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/yaml")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Client.Validate: do request: %v", err)
	}
	return resp
}

func (c *Client) CollectionFollow(ns string) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.url("/v0/collection/"+url.PathEscape(ns)+"/follow"), nil)
	if err != nil {
		c.t.Fatalf("Client.CollectionFollow: create request: %v", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Client.CollectionFollow: do request: %v", err)
	}
	return resp
}

func (c *Client) CollectionUnfollow(ns string) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodDelete, c.url("/v0/collection/"+url.PathEscape(ns)+"/follow"), nil)
	if err != nil {
		c.t.Fatalf("Client.CollectionUnfollow: create request: %v", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Client.CollectionUnfollow: do request: %v", err)
	}
	return resp
}

func (c *Client) CollectionGet(ns string) *http.Response {
	c.t.Helper()
	resp, err := c.http.Get(c.url("/v0/collection/" + url.PathEscape(ns)))
	if err != nil {
		c.t.Fatalf("Client.CollectionGet: do request: %v", err)
	}
	return resp
}

func (c *Client) CollectionList(followed *bool) *http.Response {
	c.t.Helper()
	u := c.url("/v0/collection")
	if followed != nil {
		if *followed {
			u += "?followed=true"
		} else {
			u += "?followed=false"
		}
	}
	resp, err := c.http.Get(u)
	if err != nil {
		c.t.Fatalf("Client.CollectionList: do request: %v", err)
	}
	return resp
}

func (c *Client) CollectionSeedManifest(ns string, body []byte) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.url("/v0/collection/"+url.PathEscape(ns)+"/manifest"), bytes.NewReader(body))
	if err != nil {
		c.t.Fatalf("Client.CollectionSeedManifest: create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/yaml")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Client.CollectionSeedManifest: do request: %v", err)
	}
	return resp
}

func (c *Client) CollectionGetManifest(ns string) *http.Response {
	c.t.Helper()
	resp, err := c.http.Get(c.url("/v0/collection/" + url.PathEscape(ns) + "/manifest"))
	if err != nil {
		c.t.Fatalf("Client.CollectionGetManifest: do request: %v", err)
	}
	return resp
}

func (c *Client) CollectionValidateManifest(ns string, body []byte) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodPost, c.url("/v0/collection/"+url.PathEscape(ns)+"/manifest/validate"), bytes.NewReader(body))
	if err != nil {
		c.t.Fatalf("Client.CollectionValidateManifest: create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/yaml")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("Client.CollectionValidateManifest: do request: %v", err)
	}
	return resp
}

// DialRuntime opens a WebSocket connection to the arrow runtime stream.
func (c *Client) DialRuntime(ns string) (*websocket.Conn, error) {
	c.t.Helper()
	wsURL := strings.Replace(c.baseURL, "http://", "ws://", 1)
	wsURL += "/v0/runtime/" + url.PathEscape(ns)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	return conn, err
}

func (c *Client) url(path string) string {
	return c.baseURL + path
}

// MustStatus asserts resp.StatusCode == want, logs the body on failure, and closes the body.
func MustStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	defer resp.Body.Close()
	if resp.StatusCode != want {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status %d, got %d\nbody: %s", want, resp.StatusCode, body)
	}
}
