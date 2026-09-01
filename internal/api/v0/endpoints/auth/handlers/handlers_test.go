package auth_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	authhandlers "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/auth/handlers"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/domain/auth"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func setup(svc *mocks.AuthService) *gin.Engine {
	h := authhandlers.New(svc)
	r := gin.New()
	r.POST("/v0/auth/pairing", h.GeneratePairingCode)
	r.POST("/v0/auth/pairing/redeem", h.Redeem)
	r.GET("/v0/auth/devices", h.ListDevices)
	r.DELETE("/v0/auth/devices/:id", h.RevokeDevice)
	return r
}

func TestGeneratePairingCode_Success(t *testing.T) {
	expiresAt := time.Now().Add(5 * time.Minute)
	svc := &mocks.AuthService{GenerateCode: "482913", GenerateExpiresAt: expiresAt}
	r := setup(svc)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v0/auth/pairing", nil))

	require.Equal(t, http.StatusCreated, w.Code)

	var body struct {
		Success bool                  `json:"success"`
		Data    apidto.PairingCodeDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.Success)
	assert.Equal(t, "482913", body.Data.Code)
}

func TestGeneratePairingCode_ServiceError(t *testing.T) {
	svc := &mocks.AuthService{GenerateErr: apperrors.ErrStateViolation}
	r := setup(svc)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v0/auth/pairing", nil))

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestRedeem_Success(t *testing.T) {
	svc := &mocks.AuthService{RedeemToken: "session-token"}
	r := setup(svc)

	body, err := json.Marshal(apidto.RedeemPairingCodeRequestDTO{Code: "482913", DeviceID: "dev-1", Label: "laptop"})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v0/auth/pairing/redeem", bytes.NewReader(body)))

	require.Equal(t, http.StatusCreated, w.Code)
	assert.Equal(t, "482913", svc.RedeemArgs.Code)
	assert.Equal(t, "dev-1", svc.RedeemArgs.DeviceID)
	assert.Equal(t, "laptop", svc.RedeemArgs.Label)

	var respBody struct {
		Data apidto.SessionTokenDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &respBody))
	assert.Equal(t, "session-token", respBody.Data.Token)
}

func TestRedeem_MalformedBody_Returns400(t *testing.T) {
	r := setup(&mocks.AuthService{})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v0/auth/pairing/redeem", bytes.NewReader([]byte("not json"))))

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRedeem_MissingFields_Returns400(t *testing.T) {
	testCases := []apidto.RedeemPairingCodeRequestDTO{
		{Code: "", DeviceID: "dev-1", Label: "laptop"},
		{Code: "482913", DeviceID: "", Label: "laptop"},
		{Code: "482913", DeviceID: "dev-1", Label: ""},
		{Code: "  ", DeviceID: "dev-1", Label: "laptop"},
	}

	for _, tc := range testCases {
		r := setup(&mocks.AuthService{})
		body, err := json.Marshal(tc)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v0/auth/pairing/redeem", bytes.NewReader(body)))

		assert.Equal(t, http.StatusBadRequest, w.Code, "%+v", tc)
	}
}

func TestRedeem_ServiceError(t *testing.T) {
	svc := &mocks.AuthService{RedeemErr: apperrors.ErrInvalidPairingCode}
	r := setup(svc)

	body, err := json.Marshal(apidto.RedeemPairingCodeRequestDTO{Code: "000000", DeviceID: "dev-1", Label: "laptop"})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v0/auth/pairing/redeem", bytes.NewReader(body)))

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestListDevices_Success(t *testing.T) {
	svc := &mocks.AuthService{ListDevicesResult: []auth.Device{
		{ID: "dev-1", Label: "laptop", State: auth.DeviceStateActive},
	}}
	r := setup(svc)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v0/auth/devices", nil))

	require.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data []apidto.DeviceDTO `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	assert.Equal(t, "dev-1", body.Data[0].ID)
}

func TestListDevices_ServiceError(t *testing.T) {
	svc := &mocks.AuthService{ListDevicesErr: apperrors.ErrStateViolation}
	r := setup(svc)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v0/auth/devices", nil))

	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestRevokeDevice_Success(t *testing.T) {
	svc := &mocks.AuthService{}
	r := setup(svc)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/v0/auth/devices/dev-1", nil))

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "dev-1", svc.RevokeDeviceID)
}

func TestRevokeDevice_NotFound(t *testing.T) {
	svc := &mocks.AuthService{RevokeDeviceErr: apperrors.ErrNotFound}
	r := setup(svc)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/v0/auth/devices/missing", nil))

	assert.Equal(t, http.StatusNotFound, w.Code)
}
