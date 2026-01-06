package apilibs

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	errors "github.com/rabbytesoftware/quiver/internal/core/errs"
)

func TestNewApiResponse(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	resp := NewApiResponse(c)

	if resp == nil {
		t.Fatal("NewApiResponse returned nil")
	}
	if resp.c != c {
		t.Fatal("expected c to be set")
	}
	if resp.start.IsZero() {
		t.Fatal("expected start to be set")
	}
}

func TestToResponse_StatusSuccess(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	resp := NewApiResponse(c)
	resp.ToResponse(ResponseInput{
		StatusSuccess: 201,
	})

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
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	resp := NewApiResponse(c)
	resp.ToResponse(ResponseInput{
		Error: &errors.Error{
			Code:    errors.InvalidRequestCode,
			Message: "test error",
		},
	})

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
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	resp := NewApiResponse(c)
	resp.ToResponse(ResponseInput{
		Warnings: []error{
			errors.InvalidRequest("warning 1"),
		},
	})

	if w.Code != 206 {
		t.Fatalf("expected status 206, got %d", w.Code)
	}

	// Just verify the response is valid JSON without unmarshaling warnings
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

	resp := NewApiResponse(c)
	resp.ToResponse(ResponseInput{})

	if w.Code != 200 {
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

func TestPayloadBody_ImplementsPayload(t *testing.T) {
	pb := PayloadBody[string]{Data: "test"}

	var p Payload = pb
	// If the assignment succeeds, PayloadBody implements Payload
	_ = p
}

func TestToResponse_WithPayload(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	resp := NewApiResponse(c)
	payload := PayloadBody[string]{Data: "test data"}

	resp.ToResponse(ResponseInput{
		StatusSuccess: 200,
		Payload:       payload,
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
}

func TestToResponse_ErrorAndWarningsExclusive(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	resp := NewApiResponse(c)
	resp.ToResponse(ResponseInput{
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

	resp := NewApiResponse(c)
	payload := PayloadBody[string]{Data: "test data"}

	resp.ToResponse(ResponseInput{
		Error:   &errors.Error{Code: 400, Message: "error"},
		Payload: payload,
	})

	if w.Code != 400 {
		t.Fatalf("expected status 400, got %d", w.Code)
	}
}

func TestToResponse_MultipleWarnings(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	resp := NewApiResponse(c)
	resp.ToResponse(ResponseInput{
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

	resp := NewApiResponse(c)
	resp.ToResponse(ResponseInput{
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
		{"nil", nil},
		{"map", map[string]interface{}{"key": "value"}},
		{"slice", []string{"a", "b", "c"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			resp := NewApiResponse(c)
			payload := PayloadBody[interface{}]{Data: tc.data}

			resp.ToResponse(ResponseInput{
				StatusSuccess: 200,
				Payload:       payload,
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

	resp := NewApiResponse(c)
	resp.ToResponse(ResponseInput{StatusSuccess: 200})

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
