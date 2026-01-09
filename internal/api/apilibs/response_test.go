package apilibs

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	errors "github.com/rabbytesoftware/quiver/internal/core/errs"
)

func TestToResponse_StatusSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("request_start_time", time.Now().Add(-10*time.Millisecond))

	ToResponse(c, ResponseInput[string]{
		StatusSuccess: 201,
	})

	if w.Code != 201 {
		t.Fatalf("expected status 201, got %d", w.Code)
	}

	var got Response[string]
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if !got.Success {
		t.Fatal("expected Success true")
	}
}

func TestToResponse_WithError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("request_start_time", time.Now().Add(-10*time.Millisecond))

	ToResponse(c, ResponseInput[string]{
		Error: &errors.Error{
			Code:    errors.InvalidRequestCode,
			Message: "test error",
		},
	})

	if w.Code != 400 {
		t.Fatalf("expected status 400, got %d", w.Code)
	}

	var got Response[string]
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
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("request_start_time", time.Now().Add(-10*time.Millisecond))

	ToResponse(c, ResponseInput[string]{
		Warnings: []error{
			errors.InvalidRequest("warning 1"),
		},
	})

	if w.Code != 206 {
		t.Fatalf("expected status 206, got %d", w.Code)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if success, ok := got["success"].(bool); !ok || success {
		t.Fatal("expected Success false when warnings present")
	}
	if warnings, ok := got["warnings"]; !ok || warnings == nil {
		t.Fatal("expected Warnings to be set")
	}
}

func TestToResponse_Default(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("request_start_time", time.Now().Add(-10*time.Millisecond))

	ToResponse(c, ResponseInput[string]{})

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var got Response[string]
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if !got.Success {
		t.Fatal("expected Success true for default case")
	}
}

func TestToResponse_WithPayload(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("request_start_time", time.Now().Add(-10*time.Millisecond))

	ToResponse(c, ResponseInput[string]{
		StatusSuccess: 200,
		Payload:       "test data",
	})

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var got Response[string]
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Payload != "test data" {
		t.Fatal("expected Payload to be set")
	}
}

func TestToResponse_ErrorAndWarningsExclusive(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("request_start_time", time.Now().Add(-10*time.Millisecond))

	ToResponse(c, ResponseInput[string]{
		Error: &errors.Error{
			Code:    errors.InvalidRequestCode,
			Message: "test error",
		},
		Warnings: []error{
			errors.InvalidRequest("warning 1"),
		},
	})

	// Error takes precedence
	if w.Code != 400 {
		t.Fatalf("expected status 400 (error precedence), got %d", w.Code)
	}
}

func TestToResponse_WithPayloadAndError(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("request_start_time", time.Now().Add(-10*time.Millisecond))

	ToResponse(c, ResponseInput[string]{
		Error:   &errors.Error{Code: 400, Message: "error"},
		Payload: "test data",
	})

	if w.Code != 400 {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestToResponse_MultipleWarnings(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("request_start_time", time.Now().Add(-10*time.Millisecond))

	ToResponse(c, ResponseInput[string]{
		Warnings: []error{
			errors.InvalidRequest("warning 1"),
			errors.InvalidRequest("warning 2"),
			errors.InvalidRequest("warning 3"),
		},
	})

	if w.Code != 206 {
		t.Fatalf("expected status 206, got %d", w.Code)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if success, ok := got["success"].(bool); !ok || success {
		t.Fatal("expected Success false when warnings present")
	}
}

func TestToResponse_StatusSuccessWithWarnings(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("request_start_time", time.Now().Add(-10*time.Millisecond))

	ToResponse(c, ResponseInput[string]{
		StatusSuccess: 200,
		Warnings: []error{
			errors.InvalidRequest("warning 1"),
		},
	})

	// When there are warnings, status becomes 206
	if w.Code != 206 {
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
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("request_start_time", time.Now().Add(-10*time.Millisecond))

			ToResponse(c, ResponseInput[interface{}]{
				StatusSuccess: 200,
				Payload:       tc.data,
			})

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
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("request_start_time", time.Now().Add(-10*time.Millisecond))

	ToResponse(c, ResponseInput[string]{StatusSuccess: 200})

	var got Response[string]
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
	// No request_start_time set

	ToResponse(c, ResponseInput[string]{
		StatusSuccess: 200,
	})

	if w.Code != 200 {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var got Response[string]
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Success {
		t.Fatal("expected Success false when no request_start_time")
	}
}

func TestToResponse_SuccessFalseWhenWarnings(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("request_start_time", time.Now().Add(-10*time.Millisecond))

	ToResponse(c, ResponseInput[string]{
		StatusSuccess: 200,
		Warnings: []error{
			errors.InvalidRequest("warning"),
		},
	})

	var got Response[string]
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Success {
		t.Fatal("expected Success false when warnings present")
	}
}

func TestToResponse_StatusCodeVariations(t *testing.T) {
	testCases := []struct {
		input              ResponseInput[string]
		expectedStatusCode int
		expectedSuccess    bool
		name               string
	}{
		{
			name:               "201 Created",
			input:              ResponseInput[string]{StatusSuccess: 201},
			expectedStatusCode: 201,
			expectedSuccess:    true,
		},
		{
			name:               "204 No Content",
			input:              ResponseInput[string]{StatusSuccess: 204},
			expectedStatusCode: 204,
			expectedSuccess:    true,
		},
		{
			name:               "500 Internal Server Error",
			input:              ResponseInput[string]{Error: &errors.Error{Code: 500, Message: "error"}},
			expectedStatusCode: 500,
			expectedSuccess:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set("request_start_time", time.Now().Add(-10*time.Millisecond))

			ToResponse(c, tc.input)

			if w.Code != tc.expectedStatusCode {
				t.Fatalf("expected status %d, got %d", tc.expectedStatusCode, w.Code)
			}

			var got Response[string]
			if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}

			if got.Success != tc.expectedSuccess {
				t.Fatalf("expected Success %v, got %v", tc.expectedSuccess, got.Success)
			}
		})
	}
}
