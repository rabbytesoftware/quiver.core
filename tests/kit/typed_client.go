//go:build integration

package kit

import (
	"encoding/json"
	"testing"

	dto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
)

// apiEnvelope mirrors the JSON wrapper used by all API responses.
type apiEnvelope[T any] struct {
	Success bool    `json:"success"`
	Error   *string `json:"error"`
	Data    T       `json:"data,omitempty"`
}

// TypedClient wraps Client and decodes responses into dto.* structs.
// Use this in happy-path tests. Use Client directly only for error-path tests
// that need to inspect raw status codes without a valid body to decode.
type TypedClient struct {
	raw *Client
	t   *testing.T
}

// NewTypedClient creates a TypedClient pointed at baseURL.
func NewTypedClient(t *testing.T, baseURL string) *TypedClient {
	t.Helper()
	return &TypedClient{raw: NewClient(t, baseURL), t: t}
}

// Add adds the arrow and returns the HTTP status code.
func (tc *TypedClient) Add(ns string) int {
	resp := tc.raw.Add(ns)
	defer resp.Body.Close()
	return resp.StatusCode
}

// Remove removes the arrow and returns the HTTP status code.
func (tc *TypedClient) Remove(ns string) int {
	resp := tc.raw.Remove(ns)
	defer resp.Body.Close()
	return resp.StatusCode
}

// List returns all catalog entries and the HTTP status code.
func (tc *TypedClient) List() ([]dto.ArrowListItemDTO, int) {
	resp := tc.raw.List()
	defer resp.Body.Close()
	var env apiEnvelope[[]dto.ArrowListItemDTO]
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		tc.t.Fatalf("TypedClient.List: decode: %v", err)
	}
	return env.Data, resp.StatusCode
}

// GetDetail returns the arrow detail and the HTTP status code.
func (tc *TypedClient) GetDetail(ns string) (dto.ArrowDetailDTO, int) {
	resp := tc.raw.GetDetail(ns)
	defer resp.Body.Close()
	var env apiEnvelope[dto.ArrowDetailDTO]
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		tc.t.Fatalf("TypedClient.GetDetail: decode: %v", err)
	}
	return env.Data, resp.StatusCode
}

// Update patches the arrow manifest and returns the HTTP status code.
func (tc *TypedClient) Update(ns string, body map[string]any) int {
	resp := tc.raw.Update(ns, body)
	defer resp.Body.Close()
	return resp.StatusCode
}

// Execute triggers a lifecycle method and returns the HTTP status code.
func (tc *TypedClient) Execute(ns, method string, vars map[string]string) int {
	resp := tc.raw.Execute(ns, method, vars)
	defer resp.Body.Close()
	return resp.StatusCode
}

// Install triggers the install lifecycle and returns the HTTP status code.
func (tc *TypedClient) Install(ns string, vars map[string]string) int {
	return tc.Execute(ns, "install", vars)
}

// Uninstall triggers the uninstall lifecycle and returns the HTTP status code.
func (tc *TypedClient) Uninstall(ns string, vars map[string]string) int {
	return tc.Execute(ns, "uninstall", vars)
}

// Stop triggers the stop lifecycle and returns the HTTP status code.
func (tc *TypedClient) Stop(ns string) int {
	return tc.Execute(ns, "stop", nil)
}

// Seed seeds the arrow from a raw manifest body and returns the HTTP status code.
func (tc *TypedClient) Seed(ns string, body []byte) int {
	resp := tc.raw.Seed(ns, body)
	defer resp.Body.Close()
	return resp.StatusCode
}

// Validate validates a manifest body and returns the validation result and HTTP status code.
func (tc *TypedClient) Validate(ns string, body []byte) (dto.ValidationResultDTO, int) {
	resp := tc.raw.Validate(ns, body)
	defer resp.Body.Close()
	var env apiEnvelope[dto.ValidationResultDTO]
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		tc.t.Fatalf("TypedClient.Validate: decode: %v", err)
	}
	return env.Data, resp.StatusCode
}

// CollectionFollow follows a quiver and returns the HTTP status code.
func (tc *TypedClient) CollectionFollow(ns string) int {
	resp := tc.raw.CollectionFollow(ns)
	defer resp.Body.Close()
	return resp.StatusCode
}

// CollectionUnfollow unfollows a quiver and returns the HTTP status code.
func (tc *TypedClient) CollectionUnfollow(ns string) int {
	resp := tc.raw.CollectionUnfollow(ns)
	defer resp.Body.Close()
	return resp.StatusCode
}

// CollectionGet returns the quiver detail and the HTTP status code.
func (tc *TypedClient) CollectionGet(ns string) (dto.CollectionDetailDTO, int) {
	resp := tc.raw.CollectionGet(ns)
	defer resp.Body.Close()
	var env apiEnvelope[dto.CollectionDetailDTO]
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		tc.t.Fatalf("TypedClient.CollectionGet: decode: %v", err)
	}
	return env.Data, resp.StatusCode
}

// CollectionList returns the quiver list and the HTTP status code.
func (tc *TypedClient) CollectionList(followed *bool) ([]dto.CollectionListItemDTO, int) {
	resp := tc.raw.CollectionList(followed)
	defer resp.Body.Close()
	var env apiEnvelope[[]dto.CollectionListItemDTO]
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		tc.t.Fatalf("TypedClient.CollectionList: decode: %v", err)
	}
	return env.Data, resp.StatusCode
}

// CollectionSeedManifest seeds a quiver from a raw manifest body and returns the HTTP status code.
func (tc *TypedClient) CollectionSeedManifest(ns string, body []byte) int {
	resp := tc.raw.CollectionSeedManifest(ns, body)
	defer resp.Body.Close()
	return resp.StatusCode
}

// CollectionValidateManifest validates a quiver manifest and returns the result and HTTP status code.
func (tc *TypedClient) CollectionValidateManifest(ns string, body []byte) (dto.ValidationResultDTO, int) {
	resp := tc.raw.CollectionValidateManifest(ns, body)
	defer resp.Body.Close()
	var env apiEnvelope[dto.ValidationResultDTO]
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		tc.t.Fatalf("TypedClient.CollectionValidateManifest: decode: %v", err)
	}
	return env.Data, resp.StatusCode
}

// collectionManifestResponse mirrors domain.Collection JSON (no envelope, capital keys, no JSON tags).
type collectionManifestResponse struct {
	Meta   collectionMetaResponse    `json:"Meta"`
	Arrows []collectionArrowResponse `json:"Arrows"`
}

type collectionMetaResponse struct {
	Name        string   `json:"Name"`
	Version     string   `json:"Version"`
	Description string   `json:"Description"`
	URL         string   `json:"URL"`
	Maintainers []string `json:"Maintainers"`
	Tags        []string `json:"Tags"`
}

type collectionArrowResponse struct {
	Namespace string `json:"Namespace"`
}

// CollectionGetManifest fetches the raw cached quiver manifest and returns a decoded response and status code.
func (tc *TypedClient) CollectionGetManifest(ns string) (collectionManifestResponse, int) {
	resp := tc.raw.CollectionGetManifest(ns)
	defer resp.Body.Close()
	var m collectionManifestResponse
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		tc.t.Fatalf("TypedClient.CollectionGetManifest: decode: %v", err)
	}
	return m, resp.StatusCode
}
