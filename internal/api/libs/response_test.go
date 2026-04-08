package libs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rabbytesoftware/quiver/internal/api/libs/apierr"
)

func newTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("request_start_time", time.Now().Add(-10*time.Millisecond))
	return c, w
}

func TestToResponse_StatusSuccess(t *testing.T) {
	c, w := newTestContext()

	ToResponse(c, WithSuccessCode(201))

	if w.Code != 201 {
		t.Fatalf("expected status 201, got %d", w.Code)
	}

	var got Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if !got.Success {
		t.Fatal("expected Success true")
	}
}

func TestToResponse_WithError(t *testing.T) {
	c, w := newTestContext()

	ToResponse(c, WithError(&apierr.Error{
		Code:    apierr.InvalidRequestCode,
		Message: "test error",
	}))

	if w.Code != 400 {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var got Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Success {
		t.Fatal("expected Success false when error present")
	}
	if got.Error == nil {
		t.Fatal("expected Error to be set")
	}
}

func TestToResponse_WithWarnings(t *testing.T) {
	c, w := newTestContext()

	ToResponse(c, WithWarnings([]error{
		apierr.InvalidRequest("warning 1"),
	}))

	if w.Code != http.StatusPartialContent {
		t.Fatalf("expected status 206, got %d", w.Code)
	}

	var got Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Success {
		t.Fatal("expected Success false when warnings present")
	}
	if len(got.Warnings) == 0 {
		t.Fatal("expected Warnings to be set")
	}
}

func TestToResponse_Default(t *testing.T) {
	c, w := newTestContext()

	ToResponse(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var got Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if !got.Success {
		t.Fatal("expected Success true for default case")
	}
}

func TestToResponse_WithPayload(t *testing.T) {
	c, w := newTestContext()

	ToResponse(
		c,
		WithSuccessCode(200),
		WithPayload("test data"),
	)

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var got Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Payload != "test data" {
		t.Fatal("expected Payload to be set")
	}
}

func TestToResponse_ErrorAndWarningsExclusive(t *testing.T) {
	c, w := newTestContext()

	ToResponse(
		c,
		WithError(&apierr.Error{
			Code:    apierr.InvalidRequestCode,
			Message: "test error",
		}),
		WithWarnings([]error{
			apierr.InvalidRequest("warning 1"),
		}),
	)

	if w.Code != 400 {
		t.Fatalf("expected status 400 (error precedence), got %d", w.Code)
	}
}

func TestToResponse_WithPayloadAndError(t *testing.T) {
	c, w := newTestContext()

	ToResponse(
		c,
		WithError(&apierr.Error{Code: 400, Message: "error"}),
		WithPayload("test data"),
	)

	if w.Code != 400 {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestToResponse_MultipleWarnings(t *testing.T) {
	c, w := newTestContext()

	ToResponse(c, WithWarnings([]error{
		apierr.InvalidRequest("warning 1"),
		apierr.InvalidRequest("warning 2"),
		apierr.InvalidRequest("warning 3"),
	}))

	if w.Code != http.StatusPartialContent {
		t.Fatalf("expected status 206, got %d", w.Code)
	}

	var got Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Success {
		t.Fatal("expected Success false when warnings present")
	}
}

func TestToResponse_StatusSuccessWithWarnings(t *testing.T) {
	c, w := newTestContext()

	ToResponse(
		c,
		WithSuccessCode(200),
		WithWarnings([]error{
			apierr.InvalidRequest("warning 1"),
		}),
	)

	if w.Code != http.StatusPartialContent {
		t.Fatalf("expected status 206 (warnings override), got %d", w.Code)
	}
}

func TestResponsePayloadWithDifferentTypes(t *testing.T) {
	testCases := []struct {
		name string
		data interface{}
	}{
		{"string", "test string"},
		{"int", 42},
		{"float", 3.14},
		{"bool", true},
		{"map", map[string]interface{}{"key": "value"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newTestContext()

			ToResponse(
				c,
				WithSuccessCode(200),
				WithPayload(tc.data),
			)

			if w.Code != 200 {
				t.Fatalf("expected status 200, got %d", w.Code)
			}

			var got map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}

			if payload, ok := got["payload"]; !ok || payload == nil {
				t.Fatal("expected Payload to be set")
			}
		})
	}
}

func TestResponseTimestampAndResponseTime(t *testing.T) {
	c, w := newTestContext()

	ToResponse(c, WithSuccessCode(200))

	var got Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Timestamp.IsZero() {
		t.Fatal("expected Timestamp to be set")
	}

	if got.ResponseTime == "" {
		t.Fatal("expected ResponseTime to be set")
	}
}

func TestToResponse_WithoutRequestStartTime(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	ToResponse(c, WithSuccessCode(200))

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var got Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if !got.Success {
		t.Fatal("expected Success true even without request_start_time")
	}
}

func TestToResponse_SuccessFalseWhenWarnings(t *testing.T) {
	c, w := newTestContext()

	ToResponse(
		c,
		WithSuccessCode(200),
		WithWarnings([]error{
			apierr.InvalidRequest("warning"),
		}),
	)

	var got Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Success {
		t.Fatal("expected Success false when warnings present")
	}
}

func TestToResponse_StatusCodeVariations(t *testing.T) {
	testCases := []struct {
		name               string
		opts               []ResponseOption
		expectedStatusCode int
		expectedSuccess    bool
	}{
		{
			name:               "201 Created",
			opts:               []ResponseOption{WithSuccessCode(201)},
			expectedStatusCode: 201,
			expectedSuccess:    true,
		},
		{
			name:               "204 No Content",
			opts:               []ResponseOption{WithSuccessCode(204)},
			expectedStatusCode: 204,
			expectedSuccess:    true,
		},
		{
			name: "500 Internal Server Error",
			opts: []ResponseOption{
				WithError(&apierr.Error{Code: 500, Message: "error"}),
			},
			expectedStatusCode: 500,
			expectedSuccess:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			c, w := newTestContext()

			ToResponse(c, tc.opts...)

			if w.Code != tc.expectedStatusCode {
				t.Fatalf("expected status %d, got %d", tc.expectedStatusCode, w.Code)
			}

			if tc.expectedStatusCode == http.StatusNoContent {
				if w.Body.Len() != 0 {
					t.Fatal("expected empty body for 204 No Content")
				}
				return
			}

			var got Response
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}

			if got.Success != tc.expectedSuccess {
				t.Fatalf("expected Success %v, got %v", tc.expectedSuccess, got.Success)
			}
		})
	}
}
