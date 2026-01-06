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

	var got Response
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Payload == nil {
		t.Fatal("expected Payload to be set")
	}
}
