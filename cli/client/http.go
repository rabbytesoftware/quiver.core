package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// HTTPClient implements QuiverClient over HTTP REST and WebSocket.
type HTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewHTTPClient returns an HTTPClient targeting baseURL (e.g. "http://localhost:8080").
func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// apiEnvelope is the server's standard response wrapper.
type apiEnvelope struct {
	Success bool            `json:"success"`
	Error   *string         `json:"error"`
	Data    json.RawMessage `json:"data"`
}

// getJSON sends GET to path and decodes the data field as T.
func getJSON[T any](ctx context.Context, c *HTTPClient, path string) (T, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		var zero T
		return zero, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		var zero T
		return zero, err
	}
	defer resp.Body.Close()
	return decodeEnvelope[T](resp.Body)
}

// decodeEnvelope decodes an apiEnvelope and unmarshals data as T.
func decodeEnvelope[T any](r io.Reader) (T, error) {
	var env apiEnvelope
	if err := json.NewDecoder(r).Decode(&env); err != nil {
		var zero T
		return zero, err
	}
	if !env.Success {
		var zero T
		msg := "request failed"
		if env.Error != nil {
			msg = *env.Error
		}
		return zero, fmt.Errorf("%s", msg)
	}
	var result T
	if err := json.Unmarshal(env.Data, &result); err != nil {
		return result, err
	}
	return result, nil
}

// mutate sends a mutation request (no body or JSON body) and returns an error on HTTP >= 400.
func (c *HTTPClient) mutate(ctx context.Context, method, path string, body any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var env apiEnvelope
		_ = json.NewDecoder(resp.Body).Decode(&env)
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if env.Error != nil {
			msg = *env.Error
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// wsURL converts the HTTP base URL to its WebSocket equivalent.
func (c *HTTPClient) wsURL(path string) string {
	switch {
	case strings.HasPrefix(c.baseURL, "https://"):
		return "wss://" + strings.TrimPrefix(c.baseURL, "https://") + path
	default:
		return "ws://" + strings.TrimPrefix(c.baseURL, "http://") + path
	}
}

// ns returns the URL-path-escaped form of a namespace.
// e.g. "github.com/foo/bar" → "github.com%2Ffoo%2Fbar"
func ns(namespace string) string {
	// url.PathEscape doesn't encode '@' (a sub-delimiter), but version-pinned
	// namespaces like "github.com/foo/bar@v1.0.0" must encode it for the server.
	return strings.ReplaceAll(url.PathEscape(namespace), "@", "%40")
}

// --- Arrow catalog ---

func (c *HTTPClient) ArrowList(ctx context.Context, userInstalled bool) ([]ArrowListItem, error) {
	return getJSON[[]ArrowListItem](ctx, c, fmt.Sprintf("/v0/arrow?user_installed=%v", userInstalled))
}

func (c *HTTPClient) ArrowGet(ctx context.Context, namespace string) (*ArrowDetail, error) {
	result, err := getJSON[ArrowDetail](ctx, c, "/v0/arrow/"+ns(namespace))
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *HTTPClient) ArrowGetManifest(ctx context.Context, namespace string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v0/arrow/"+ns(namespace)+"/manifest", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var env apiEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if !env.Success {
		msg := "request failed"
		if env.Error != nil {
			msg = *env.Error
		}
		return nil, fmt.Errorf("%s", msg)
	}
	return env.Data, nil
}

func (c *HTTPClient) ArrowAdd(ctx context.Context, namespace string) error {
	return c.mutate(ctx, http.MethodPost, "/v0/arrow/"+ns(namespace), nil)
}

func (c *HTTPClient) ArrowUpdate(ctx context.Context, namespace string) error {
	return c.mutate(ctx, http.MethodPatch, "/v0/arrow/"+ns(namespace), nil)
}

func (c *HTTPClient) ArrowRemove(ctx context.Context, namespace string) error {
	return c.mutate(ctx, http.MethodDelete, "/v0/arrow/"+ns(namespace), nil)
}

func (c *HTTPClient) ArrowSeed(ctx context.Context, namespace string, manifest []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v0/arrow/"+ns(namespace)+"/manifest", bytes.NewReader(manifest))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		var env apiEnvelope
		_ = json.NewDecoder(resp.Body).Decode(&env)
		msg := fmt.Sprintf("HTTP %d", resp.StatusCode)
		if env.Error != nil {
			msg = *env.Error
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func (c *HTTPClient) ArrowValidate(ctx context.Context, namespace string, manifest []byte) (*ValidationResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v0/arrow/"+ns(namespace)+"/manifest/validate", bytes.NewReader(manifest))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-yaml")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Server returns 200 for valid, 422 for invalid — both use the same envelope shape.
	var env apiEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	var result ValidationResult
	if err := json.Unmarshal(env.Data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// --- Runtime lifecycle ---

// postRuntime fires POST /v0/runtime/:ns/<method> with optional variables.
func (c *HTTPClient) postRuntime(ctx context.Context, namespace, method string, vars map[string]string) error {
	type body struct {
		Variables map[string]string `json:"variables,omitempty"`
	}
	path := "/v0/runtime/" + ns(namespace) + "/" + url.PathEscape(method)
	return c.mutate(ctx, http.MethodPost, path, body{Variables: vars})
}

// lifecycle fires POST then opens the WS stream for namespace.
func (c *HTTPClient) lifecycle(ctx context.Context, namespace, method string, vars map[string]string, stopFn func(ArrowRuntime, bool) bool) (<-chan ArrowRuntime, error) {
	if err := c.postRuntime(ctx, namespace, method, vars); err != nil {
		return nil, err
	}
	return pump(ctx, c.wsURL("/v0/runtime/"+ns(namespace)), stopFn)
}

func (c *HTTPClient) Install(ctx context.Context, namespace string, vars map[string]string) (<-chan ArrowRuntime, error) {
	return c.lifecycle(ctx, namespace, "install", vars, terminalInstall)
}

func (c *HTTPClient) Uninstall(ctx context.Context, namespace string, vars map[string]string) (<-chan ArrowRuntime, error) {
	return c.lifecycle(ctx, namespace, "uninstall", vars, terminalUninstall)
}

func (c *HTTPClient) Run(ctx context.Context, namespace string, vars map[string]string) (<-chan ArrowRuntime, error) {
	return c.lifecycle(ctx, namespace, "execute", vars, terminalReady)
}

func (c *HTTPClient) Stop(ctx context.Context, namespace string) (<-chan ArrowRuntime, error) {
	return c.lifecycle(ctx, namespace, "stop", nil, terminalReady)
}

func (c *HTTPClient) Update(ctx context.Context, namespace string) (<-chan ArrowRuntime, error) {
	return c.lifecycle(ctx, namespace, "update", nil, terminalReady)
}

func (c *HTTPClient) RunMethod(ctx context.Context, namespace, method string, vars map[string]string) (<-chan ArrowRuntime, error) {
	return c.lifecycle(ctx, namespace, method, vars, terminalCustomMethod)
}

// --- Runtime observation ---

func (c *HTTPClient) RuntimeGet(ctx context.Context, namespace string) (*ArrowRuntime, error) {
	stopAfterOne := func(_ ArrowRuntime, _ bool) bool { return true }
	ch, err := pump(ctx, c.wsURL("/v0/runtime/"+ns(namespace)), stopAfterOne)
	if err != nil {
		return nil, err
	}
	rt, ok := <-ch
	if !ok {
		return nil, fmt.Errorf("no runtime snapshot received for %s", namespace)
	}
	return &rt, nil
}

func (c *HTTPClient) RuntimeList(ctx context.Context) ([]ArrowRuntime, error) {
	// /v0/runtime broadcasts all runtime updates; read the first snapshot.
	stopAfterOne := func(_ ArrowRuntime, _ bool) bool { return true }
	ch, err := pump(ctx, c.wsURL("/v0/runtime"), stopAfterOne)
	if err != nil {
		return nil, err
	}
	rt, ok := <-ch
	if !ok {
		return nil, nil
	}
	return []ArrowRuntime{rt}, nil
}

func (c *HTTPClient) WatchRuntime(ctx context.Context, namespace string) (<-chan ArrowRuntime, error) {
	return pump(ctx, c.wsURL("/v0/runtime/"+ns(namespace)), neverStop)
}

// --- Collections ---
// The server uses /quiver for what the CLI calls Collection.

func (c *HTTPClient) CollectionList(ctx context.Context) ([]Collection, error) {
	return getJSON[[]Collection](ctx, c, "/v0/quiver")
}

func (c *HTTPClient) CollectionGet(ctx context.Context, namespace string) (*Collection, error) {
	result, err := getJSON[Collection](ctx, c, "/v0/quiver/"+ns(namespace))
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *HTTPClient) CollectionAdd(ctx context.Context, namespace string) error {
	return c.mutate(ctx, http.MethodPost, "/v0/quiver/"+ns(namespace), nil)
}

func (c *HTTPClient) CollectionUpdate(ctx context.Context, namespace string) error {
	return c.mutate(ctx, http.MethodPatch, "/v0/quiver/"+ns(namespace), nil)
}

func (c *HTTPClient) CollectionRemove(ctx context.Context, namespace string) error {
	return c.mutate(ctx, http.MethodDelete, "/v0/quiver/"+ns(namespace), nil)
}

// --- System ---

func (c *HTTPClient) Health(ctx context.Context) (*HealthStatus, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v0/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var hs HealthStatus
	if err := json.NewDecoder(resp.Body).Decode(&hs); err != nil {
		return nil, err
	}
	return &hs, nil
}

// Compile-time proof that HTTPClient satisfies QuiverClient.
var _ QuiverClient = (*HTTPClient)(nil)
