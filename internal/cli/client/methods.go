package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
)

// ─── system ──────────────────────────────────────────────────────────────────

// Health pings /v0/health. A nil return means the daemon answered ok.
func (c *Client) Health(ctx context.Context) error {
	var out struct {
		Status string `json:"status"`
	}
	if err := c.doRaw(ctx, "/v0/health", &out); err != nil {
		return err
	}
	if out.Status != "ok" {
		return fmt.Errorf("client: daemon unhealthy: status %q", out.Status)
	}
	return nil
}

// VersionsInfo is the GET /versions payload.
type VersionsInfo struct {
	Version string `json:"version"`
	BuildID string `json:"build_id"`
	API     struct {
		Supported []string `json:"supported"`
		Latest    string   `json:"latest"`
	} `json:"api"`
}

// Versions returns daemon build info and supported API versions.
func (c *Client) Versions(ctx context.Context) (VersionsInfo, error) {
	var out VersionsInfo
	err := c.do(ctx, http.MethodGet, "/versions", nil, &out)
	return out, err
}

// ─── arrows ──────────────────────────────────────────────────────────────────

// ListArrows returns catalog arrows. userInstalled filters when non-nil.
func (c *Client) ListArrows(
	ctx context.Context,
	userInstalled *bool,
) ([]apidto.ArrowListItemDTO, error) {
	path := "/v0/arrow"
	if userInstalled != nil {
		path += "?user_installed=" + strconv.FormatBool(*userInstalled)
	}
	var out []apidto.ArrowListItemDTO
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}

// GetArrow returns the detail view for one arrow.
func (c *Client) GetArrow(
	ctx context.Context,
	ns string,
) (apidto.ArrowDetailDTO, error) {
	var out apidto.ArrowDetailDTO
	err := c.do(ctx, http.MethodGet, "/v0/arrow/"+encodeNS(ns), nil, &out)
	return out, err
}

// GetArrowManifest returns the compiled manifest as raw JSON.
func (c *Client) GetArrowManifest(
	ctx context.Context,
	ns string,
) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.do(ctx, http.MethodGet, "/v0/arrow/"+encodeNS(ns)+"/manifest", nil, &out)
	return out, err
}

// AddArrow registers an arrow in the catalog.
func (c *Client) AddArrow(ctx context.Context, ns string) error {
	return c.do(ctx, http.MethodPost, "/v0/arrow/"+encodeNS(ns), nil, nil)
}

// RemoveArrow deletes an arrow from the catalog.
func (c *Client) RemoveArrow(ctx context.Context, ns string) error {
	return c.do(ctx, http.MethodDelete, "/v0/arrow/"+encodeNS(ns), nil, nil)
}

// RefreshArrow re-fetches the arrow's manifest from its source, replacing the
// stored (and cached) copy.
func (c *Client) RefreshArrow(ctx context.Context, ns string) error {
	return c.do(ctx, http.MethodPatch, "/v0/arrow/"+encodeNS(ns), nil, nil)
}

// SeedArrowManifest registers an arrow from raw manifest bytes.
func (c *Client) SeedArrowManifest(
	ctx context.Context,
	ns string,
	manifest []byte,
) error {
	return c.do(ctx, http.MethodPost, "/v0/arrow/"+encodeNS(ns)+"/manifest", manifest, nil)
}

// ValidateArrowManifest validates raw manifest bytes without applying them.
func (c *Client) ValidateArrowManifest(
	ctx context.Context,
	ns string,
	manifest []byte,
) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.do(ctx, http.MethodPost, "/v0/arrow/"+encodeNS(ns)+"/manifest/validate", manifest, &out)
	return out, err
}

// ─── collections ─────────────────────────────────────────────────────────────

// ListCollections returns followed collections.
func (c *Client) ListCollections(
	ctx context.Context,
) ([]apidto.CollectionListItemDTO, error) {
	var out []apidto.CollectionListItemDTO
	err := c.do(ctx, http.MethodGet, "/v0/collection", nil, &out)
	return out, err
}

// GetCollection returns the detail view for one collection.
func (c *Client) GetCollection(
	ctx context.Context,
	ns string,
) (apidto.CollectionDetailDTO, error) {
	var out apidto.CollectionDetailDTO
	err := c.do(ctx, http.MethodGet, "/v0/collection/"+encodeNS(ns), nil, &out)
	return out, err
}

// FollowCollection follows a collection.
func (c *Client) FollowCollection(ctx context.Context, ns string) error {
	return c.do(ctx, http.MethodPost, "/v0/collection/"+encodeNS(ns)+"/follow", nil, nil)
}

// UnfollowCollection unfollows a collection.
func (c *Client) UnfollowCollection(ctx context.Context, ns string) error {
	return c.do(ctx, http.MethodDelete, "/v0/collection/"+encodeNS(ns)+"/follow", nil, nil)
}

// UpdateCollection re-resolves the collection manifest from its git source.
func (c *Client) UpdateCollection(ctx context.Context, ns string) error {
	return c.do(ctx, http.MethodPost, "/v0/collection/"+encodeNS(ns)+"/manifest", nil, nil)
}

// SeedCollectionManifest registers a collection from raw manifest bytes.
func (c *Client) SeedCollectionManifest(
	ctx context.Context,
	ns string,
	manifest []byte,
) error {
	return c.do(ctx, http.MethodPost, "/v0/collection/"+encodeNS(ns)+"/manifest", manifest, nil)
}

// ─── runtime ─────────────────────────────────────────────────────────────────

// ExecuteMethod triggers a lifecycle or custom method on an arrow. The bool
// reports whether the server started asynchronous work (202) versus treating
// the call as an idempotent no-op (200) for which no runtime events will
// stream.
func (c *Client) ExecuteMethod(
	ctx context.Context,
	ns, method string,
	vars map[string]string,
) (bool, error) {
	var body []byte
	if len(vars) > 0 {
		var err error
		body, err = json.Marshal(map[string]any{"variables": vars})
		if err != nil {
			return false, fmt.Errorf("client: marshal variables: %w", err)
		}
	}
	return c.doMutation(ctx, http.MethodPost, "/v0/runtime/"+encodeNS(ns)+"/"+method, body)
}

// GetRuntime returns the runtime snapshot for one arrow.
func (c *Client) GetRuntime(
	ctx context.Context,
	ns string,
) (apidto.ArrowRuntimeDTO, error) {
	var out apidto.ArrowRuntimeDTO
	err := c.do(ctx, http.MethodGet, "/v0/runtime/"+encodeNS(ns), nil, &out)
	return out, err
}

// ListRuntimes returns runtime snapshots for every catalog arrow.
func (c *Client) ListRuntimes(
	ctx context.Context,
) ([]apidto.ArrowRuntimeDTO, error) {
	var out []apidto.ArrowRuntimeDTO
	err := c.do(ctx, http.MethodGet, "/v0/runtime", nil, &out)
	return out, err
}

// ─── auth ────────────────────────────────────────────────────────────────────

// PairingCode is a one-time code redeemable for a device bearer token.
type PairingCode struct {
	Code      string    `json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
}

// GeneratePairingCode mints a pairing code. Reachable only from the
// daemon's own host, and only meaningful when the daemon is bound to
// tcp:// — see UnauthorizedHandler's doc comment for how session wires
// this into an automatic retry.
func (c *Client) GeneratePairingCode(
	ctx context.Context,
) (PairingCode, error) {
	var out PairingCode
	err := c.do(ctx, http.MethodPost, "/v0/auth/pairing", nil, &out)
	return out, err
}

// RedeemPairingCode exchanges code for a device bearer token, pairing
// deviceID under label in the same call.
func (c *Client) RedeemPairingCode(
	ctx context.Context,
	code, deviceID, label string,
) (string, error) {
	body, err := json.Marshal(struct {
		Code     string `json:"code"`
		DeviceID string `json:"device_id"`
		Label    string `json:"label"`
	}{Code: code, DeviceID: deviceID, Label: label})
	if err != nil {
		return "", fmt.Errorf("client: marshal redeem request: %w", err)
	}

	var out struct {
		Token string `json:"token"`
	}
	if err := c.do(ctx, http.MethodPost, "/v0/auth/pairing/redeem", body, &out); err != nil {
		return "", err
	}

	return out.Token, nil
}

// Device is one device paired with the daemon.
type Device struct {
	ID         string    `json:"id"`
	Label      string    `json:"label"`
	State      string    `json:"state"`
	PairedAt   time.Time `json:"paired_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

// ListDevices returns every device currently paired with the daemon.
func (c *Client) ListDevices(
	ctx context.Context,
) ([]Device, error) {
	var out []Device
	err := c.do(ctx, http.MethodGet, "/v0/auth/devices", nil, &out)
	return out, err
}

// RevokeDevice revokes one paired device's credential.
func (c *Client) RevokeDevice(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/v0/auth/devices/"+encodeNS(id), nil, nil)
}
