package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rabbytesoftware/quiver.core/internal/api/libs"
	"github.com/rabbytesoftware/quiver.core/internal/api/libs/apierr"
	apidto "github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
)

type Handlers struct {
	svc usecases.AuthUsecase
}

func New(svc usecases.AuthUsecase) *Handlers {
	return &Handlers{svc: svc}
}

// GeneratePairingCode mints a new one-time device-pairing code.
//
// @Summary      Generate a device-pairing code
// @Description  Mints a one-time code quiver.desktop redeems to pair with this daemon. Reachable only from the daemon's own host, and only when the daemon is bound to tcp://.
// @Tags         auth
// @Produce      json
// @Success      201  {object}  libs.QueryResponse{data=apidto.PairingCodeDTO}  "Pairing code generated"
// @Failure      403  {object}  libs.ErrResponse                                "Not reachable from this host"
// @Failure      500  {object}  libs.ErrResponse                                "Internal error"
// @Router       /auth/pairing [post]
func (h *Handlers) GeneratePairingCode(c *gin.Context) {
	code, expiresAt, err := h.svc.GeneratePairingCode(c.Request.Context())
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, "")
		return
	}

	libs.WriteQueryWithStatus(c, http.StatusCreated, apidto.PairingCodeDTO{Code: code, ExpiresAt: expiresAt})
}

// Redeem exchanges a valid pairing code for a session token.
//
// @Summary      Redeem a pairing code
// @Description  Exchanges a valid, unexpired pairing code for a session token, pairing the given device. The token is returned exactly once.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      apidto.RedeemPairingCodeRequestDTO             true  "Pairing code, a client-generated device id, and a human label"
// @Success      201   {object}  libs.QueryResponse{data=apidto.SessionTokenDTO}  "Session token issued"
// @Failure      400   {object}  libs.ErrResponse                                 "Invalid or expired pairing code"
// @Failure      429   {object}  libs.ErrResponse                                 "Too many attempts"
// @Router       /auth/pairing/redeem [post]
func (h *Handlers) Redeem(c *gin.Context) {
	var req apidto.RedeemPairingCodeRequestDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		libs.WriteErr(c, http.StatusBadRequest, "body must be a json object with code, device_id and label", "")
		return
	}

	code := strings.TrimSpace(req.Code)
	deviceID := strings.TrimSpace(req.DeviceID)
	label := strings.TrimSpace(req.Label)
	if code == "" || deviceID == "" || label == "" {
		libs.WriteErr(c, http.StatusBadRequest, "code, device_id and label are required", "")
		return
	}

	token, err := h.svc.Redeem(c.Request.Context(), code, deviceID, label)
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, "")
		return
	}

	libs.WriteQueryWithStatus(c, http.StatusCreated, apidto.SessionTokenDTO{Token: token})
}

// ListDevices lists every device currently paired with this daemon.
//
// @Summary      List paired devices
// @Description  Reachable only from the daemon's own host, and only when the daemon is bound to tcp://.
// @Tags         auth
// @Produce      json
// @Success      200  {object}  libs.QueryResponse{data=[]apidto.DeviceDTO}  "Paired devices"
// @Failure      403  {object}  libs.ErrResponse                             "Not reachable from this host"
// @Router       /auth/devices [get]
func (h *Handlers) ListDevices(c *gin.Context) {
	devices, err := h.svc.ListDevices(c.Request.Context())
	if err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, "")
		return
	}

	dtos := make([]apidto.DeviceDTO, 0, len(devices))
	for _, d := range devices {
		dtos = append(dtos, apidto.DeviceDTOFrom(d))
	}

	libs.WriteQueryOK(c, dtos)
}

// RevokeDevice revokes one paired device's credential.
//
// @Summary      Revoke a paired device
// @Description  Reachable only from the daemon's own host, and only when the daemon is bound to tcp://.
// @Tags         auth
// @Param        id  path  string  true  "Device ID"
// @Success      200  {object}  libs.MutationResponse  "Device revoked"
// @Failure      404  {object}  libs.ErrResponse       "Device not found"
// @Router       /auth/devices/{id} [delete]
func (h *Handlers) RevokeDevice(c *gin.Context) {
	id := c.Param("id")
	if err := h.svc.RevokeDevice(c.Request.Context(), id); err != nil {
		status, msg := apierr.StatusAndMessage(err)
		libs.WriteErr(c, status, msg, id)
		return
	}

	libs.WriteMutationOK(c, http.StatusOK, id)
}
