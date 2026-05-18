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
