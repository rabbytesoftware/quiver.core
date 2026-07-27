package arrows_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/mocks"
	arrows "github.com/rabbytesoftware/quiver.core/internal/api/v0/endpoints/arrows/handlers"
	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

const encodedNS = "/v0/arrow/github.com%2Fuser%2Frepo"

func setup(svc *mocks.ArrowService) (*arrows.Handlers, *gin.Engine) {
	h := arrows.New(svc)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.POST("/v0/arrow/:ns", h.Add)
	r.PATCH("/v0/arrow/:ns", h.Update)
	r.DELETE("/v0/arrow/:ns", h.Remove)
	r.GET("/v0/arrow", h.List)
	r.GET("/v0/arrow/:ns", h.GetDetail)
	r.GET("/v0/arrow/:ns/manifest", h.GetManifest)
	r.POST("/v0/arrow/:ns/manifest", h.Seed)
	r.POST("/v0/arrow/:ns/manifest/validate", h.Validate)
	return h, r
}

func TestAdd_Created(t *testing.T) {
	svc := &mocks.ArrowService{}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS, nil))
	assert.Equal(t, http.StatusCreated, w.Code)
	assertSuccess(t, w.Body.Bytes())
}

func TestAdd_ServiceError(t *testing.T) {
	svc := &mocks.ArrowService{AddErr: apperrors.ErrAlreadyExists}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, encodedNS, nil))
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestUpdate_OK(t *testing.T) {
	svc := &mocks.ArrowService{}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, encodedNS, nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestUpdate_ServiceError(t *testing.T) {
	svc := &mocks.ArrowService{UpdateErr: apperrors.ErrNotFound}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPatch, encodedNS, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestRemove_OK(t *testing.T) {
	svc := &mocks.ArrowService{}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/v0/arrow/github.com%2Fuser%2Frepo%40v1.0.0", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRemove_BareNamespace_ReturnsBadRequest(t *testing.T) {
	svc := &mocks.ArrowService{}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, encodedNS, nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRemove_DependentsExist(t *testing.T) {
	svc := &mocks.ArrowService{RemoveErr: apperrors.ErrDependentsExist}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/v0/arrow/github.com%2Fuser%2Frepo%40v1.0.0", nil))
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

func TestList_OK(t *testing.T) {
	svc := &mocks.ArrowService{
		ListResult: []models.ArrowListDTO{
			{
				Namespace: domain.Namespace("github.com/user/repo"),
				Name:      "Test",
				Versions: []models.InstalledVersionDTO{
					{Ref: "v1.0.0", Version: "v1.0.0", State: domain.ArrowStateReady},
				},
			},
		},
	}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v0/arrow", nil))
	assert.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Success bool `json:"success"`
		Data    []struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.True(t, env.Success)
	require.Len(t, env.Data, 1)
	assert.Equal(t, "github.com/user/repo", env.Data[0].Namespace)
}

func TestList_ServiceError(t *testing.T) {
	svc := &mocks.ArrowService{ListErr: apperrors.ErrFetchFailed}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v0/arrow", nil))
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestList_UserInstalled_NilWhenAbsent(t *testing.T) {
	svc := &mocks.ArrowService{}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v0/arrow", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Nil(t, svc.ListUserInstalledArg)
}

func TestList_UserInstalled_TrueParam(t *testing.T) {
	svc := &mocks.ArrowService{}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v0/arrow?user_installed=true", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, svc.ListUserInstalledArg)
	assert.True(t, *svc.ListUserInstalledArg)
}

func TestList_UserInstalled_FalseParam(t *testing.T) {
	svc := &mocks.ArrowService{}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v0/arrow?user_installed=false", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, svc.ListUserInstalledArg)
	assert.False(t, *svc.ListUserInstalledArg)
}

func TestGetDetail_OK(t *testing.T) {
	svc := &mocks.ArrowService{
		GetDetailResult: &models.ArrowDetailDTO{
			Namespace: domain.Namespace("github.com/user/repo"),
			Name:      "Test",
			State:     domain.ArrowStateReady,
		},
	}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS, nil))
	assert.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Data struct {
			Namespace string `json:"namespace"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "github.com/user/repo", env.Data.Namespace)
}

func TestGetDetail_NotFound(t *testing.T) {
	svc := &mocks.ArrowService{GetDetailErr: apperrors.ErrNotFound}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetManifest_OK(t *testing.T) {
	svc := &mocks.ArrowService{
		GetManifestResult: &models.ArrowManifestDTO{
			Namespace:   domain.Namespace("github.com/user/repo"),
			Name:        "Test",
			Description: "A test arrow",
		},
	}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS+"/manifest", nil))
	assert.Equal(t, http.StatusOK, w.Code)

	var env struct {
		Data struct {
			Namespace string `json:"namespace"`
			Name      string `json:"name"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	assert.Equal(t, "github.com/user/repo", env.Data.Namespace)
	assert.Equal(t, "Test", env.Data.Name)
}

func TestGetManifest_NotFound(t *testing.T) {
	svc := &mocks.ArrowService{GetManifestErr: apperrors.ErrNotFound}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS+"/manifest", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestGetManifest_InvalidNamespace(t *testing.T) {
	svc := &mocks.ArrowService{GetManifestErr: apperrors.ErrInvalidNamespace}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS+"/manifest", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetManifest_FetchFailed(t *testing.T) {
	svc := &mocks.ArrowService{GetManifestErr: apperrors.ErrFetchFailed}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, encodedNS+"/manifest", nil))
	assert.Equal(t, http.StatusBadGateway, w.Code)
}

func TestNamespace_PercentEncoded(t *testing.T) {
	var capturedNS domain.Namespace
	svc := &mocks.ArrowService{}
	h := arrows.New(svc)
	r := gin.New()
	r.UseRawPath = true
	r.UnescapePathValues = true
	r.GET("/v0/arrow/:ns", func(c *gin.Context) {
		capturedNS = domain.Namespace(c.Param("ns"))
		h.GetDetail(c)
	})
	svc.GetDetailResult = &models.ArrowDetailDTO{
		Namespace: domain.Namespace("github.com/user/repo"),
		Name:      "Test",
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v0/arrow/github.com%2Fuser%2Frepo", nil))
	assert.Equal(t, domain.Namespace("github.com/user/repo"), capturedNS)
}

// --- Seed handler ---

func TestSeed_Created(t *testing.T) {
	_, r := setup(&mocks.ArrowService{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, encodedNS+"/manifest", bytes.NewBufferString("manifest: arrow@v0"))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)
	assertSuccess(t, w.Body.Bytes())
}

func TestSeed_ServiceError_InvalidManifest(t *testing.T) {
	svc := &mocks.ArrowService{SeedErr: apperrors.ErrInvalidManifest}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, encodedNS+"/manifest", bytes.NewBufferString("bad yaml"))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

// A namespace with no ref is the caller's mistake, not the server's.
func TestSeed_ReflessNamespace_Returns400(t *testing.T) {
	svc := &mocks.ArrowService{SeedErr: apperrors.ErrInvalidNamespace}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, encodedNS+"/manifest", bytes.NewBufferString("manifest: arrow@v0"))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSeed_ServiceRejectsEmptyBody(t *testing.T) {
	svc := &mocks.ArrowService{SeedErr: apperrors.ErrInvalidManifest}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, encodedNS+"/manifest", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
}

// --- Validate handler ---

func TestValidate_ValidManifest_Returns200WithValidTrue(t *testing.T) {
	svc := &mocks.ArrowService{
		ValidateManifestResult: &models.ValidationResult{Valid: true},
	}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, encodedNS+"/manifest/validate", bytes.NewBufferString("manifest: arrow@v0"))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var body struct {
		Data struct {
			Valid  bool `json:"valid"`
			Errors []struct {
				Field string `json:"field"`
			} `json:"errors"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.Data.Valid)
	assert.Empty(t, body.Data.Errors)
}

func TestValidate_InvalidManifest_Returns422WithValidFalseAndErrors(t *testing.T) {
	svc := &mocks.ArrowService{
		ValidateManifestResult: &models.ValidationResult{
			Valid: false,
			Errors: []models.ValidationError{
				{Field: "lifecycle.install", Rule: "missing_pair", Message: "install requires uninstall"},
			},
		},
	}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, encodedNS+"/manifest/validate", bytes.NewBufferString("manifest: arrow@v0"))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

	var body struct {
		Success bool `json:"success"`
		Data    struct {
			Valid  bool `json:"valid"`
			Errors []struct {
				Field   string `json:"field"`
				Rule    string `json:"rule"`
				Message string `json:"message"`
			} `json:"errors"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.False(t, body.Success)
	assert.False(t, body.Data.Valid)
	require.Len(t, body.Data.Errors, 1)
	assert.Equal(t, "lifecycle.install", body.Data.Errors[0].Field)
	assert.Equal(t, "missing_pair", body.Data.Errors[0].Rule)
}

func TestValidate_ServiceError_Returns500(t *testing.T) {
	svc := &mocks.ArrowService{ValidateManifestErr: errors.New("translator failed")}
	_, r := setup(svc)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, encodedNS+"/manifest/validate", bytes.NewBufferString("not yaml"))
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestSeed_ReadBodyError_Returns400(t *testing.T) {
	_, r := setup(&mocks.ArrowService{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, encodedNS+"/manifest", &errorReader{})
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestValidate_ReadBodyError_Returns400(t *testing.T) {
	_, r := setup(&mocks.ArrowService{})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, encodedNS+"/manifest/validate", &errorReader{})
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// errorReader is an io.Reader that always returns an error.
type errorReader struct{}

func (e *errorReader) Read(_ []byte) (int, error) {
	return 0, errors.New("simulated read error")
}

func assertSuccess(t *testing.T, body []byte) {
	t.Helper()
	var env struct {
		Success bool `json:"success"`
	}
	require.NoError(t, json.Unmarshal(body, &env))
	assert.True(t, env.Success)
}
