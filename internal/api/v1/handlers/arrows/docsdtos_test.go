package arrows

import (
	"encoding/json"
	"testing"
	"time"
)

func TestErrorResponseDocsDTO_JSON(t *testing.T) {
	ts := time.Date(2026, time.January, 5, 15, 4, 5, 0, time.UTC)

	dto := ErrorResponseDocsDTO{
		Success: false,
		Error: &responseError{
			Code:    500,
			Message: "something went wrong",
			Details: map[string]interface{}{"foo": "bar"},
		},
		Timestamp:    ts,
		ResponseTime: "150ms",
	}

	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if out["success"] != false {
		t.Fatalf("unexpected success: %v", out["success"])
	}

	errObj, ok := out["error"].(map[string]interface{})
	if !ok {
		t.Fatalf("error field missing or wrong type: %T", out["error"])
	}

	if int(errObj["code"].(float64)) != 500 {
		t.Fatalf("unexpected code: %v", errObj["code"])
	}

	if errObj["message"] != "something went wrong" {
		t.Fatalf("unexpected message: %v", errObj["message"])
	}

	if out["responseTime"] != "150ms" {
		t.Fatalf("unexpected responseTime: %v", out["responseTime"])
	}

	if out["timestamp"] != "2026-01-05T15:04:05Z" {
		t.Fatalf("unexpected timestamp: %v", out["timestamp"])
	}
}

func TestWarningResponseDocsDTO_JSON(t *testing.T) {
	ts := time.Date(2026, time.January, 5, 15, 4, 5, 0, time.UTC)

	dto := WarningResponseDocsDTO[string]{
		Success:      true,
		Payload:      "ok",
		Warnings:     []string{"minor warning"},
		Timestamp:    ts,
		ResponseTime: "215ms",
	}

	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if out["success"] != true {
		t.Fatalf("unexpected success: %v", out["success"])
	}

	if out["payload"] != "ok" {
		t.Fatalf("unexpected payload: %v", out["payload"])
	}

	warnings, ok := out["warnings"].([]interface{})
	if !ok || len(warnings) != 1 || warnings[0] != "minor warning" {
		t.Fatalf("unexpected warnings: %v", out["warnings"])
	}

	if out["responseTime"] != "215ms" {
		t.Fatalf("unexpected responseTime: %v", out["responseTime"])
	}

	if out["timestamp"] != "2026-01-05T15:04:05Z" {
		t.Fatalf("unexpected timestamp: %v", out["timestamp"])
	}
}

func TestSuccessResponseDocsDTO_JSON(t *testing.T) {
	ts := time.Date(2026, time.January, 5, 15, 4, 5, 0, time.UTC)

	payload := struct {
		ID int `json:"id"`
	}{ID: 42}

	dto := SuccessResponseDocsDTO[any]{
		Success:      true,
		Payload:      payload,
		Warnings:     []string{},
		Timestamp:    ts,
		ResponseTime: "300ms",
	}

	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if out["success"] != true {
		t.Fatalf("unexpected success: %v", out["success"])
	}

	p, ok := out["payload"].(map[string]interface{})
	if !ok {
		t.Fatalf("payload missing or wrong type: %T", out["payload"])
	}
	if int(p["id"].(float64)) != 42 {
		t.Fatalf("unexpected payload id: %v", p["id"])
	}

	// warnings should be an empty array
	warnings, ok := out["warnings"].([]interface{})
	if !ok {
		t.Fatalf("warnings missing or wrong type: %T", out["warnings"])
	}
	if len(warnings) != 0 {
		t.Fatalf("expected empty warnings, got: %v", warnings)
	}

	if out["responseTime"] != "300ms" {
		t.Fatalf("unexpected responseTime: %v", out["responseTime"])
	}

	if out["timestamp"] != "2026-01-05T15:04:05Z" {
		t.Fatalf("unexpected timestamp: %v", out["timestamp"])
	}
}

func TestWarningResponseWithoutPayloadDocsDTO_JSON(t *testing.T) {
	ts := time.Date(2026, time.January, 5, 15, 4, 5, 0, time.UTC)

	dto := WarningResponseWithoutPayloadDocsDTO{
		Success:      true,
		Warnings:     []string{"minor warning"},
		Timestamp:    ts,
		ResponseTime: "215ms",
	}

	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if out["success"] != true {
		t.Fatalf("unexpected success: %v", out["success"])
	}

	warnings, ok := out["warnings"].([]interface{})
	if !ok || len(warnings) != 1 || warnings[0] != "minor warning" {
		t.Fatalf("unexpected warnings: %v", out["warnings"])
	}

	if out["responseTime"] != "215ms" {
		t.Fatalf("unexpected responseTime: %v", out["responseTime"])
	}

	if out["timestamp"] != "2026-01-05T15:04:05Z" {
		t.Fatalf("unexpected timestamp: %v", out["timestamp"])
	}
}

func TestSuccessResponseWithoutPayloadDocsDTO_JSON(t *testing.T) {
	ts := time.Date(2026, time.January, 5, 15, 4, 5, 0, time.UTC)

	dto := SuccessResponseWithoutPayloadDocsDTO{
		Success:      true,
		Warnings:     []string{},
		Timestamp:    ts,
		ResponseTime: "300ms",
	}

	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if out["success"] != true {
		t.Fatalf("unexpected success: %v", out["success"])
	}

	warnings, ok := out["warnings"].([]interface{})
	if !ok {
		t.Fatalf("warnings missing or wrong type: %T", out["warnings"])
	}
	if len(warnings) != 0 {
		t.Fatalf("expected empty warnings, got: %v", warnings)
	}

	if out["responseTime"] != "300ms" {
		t.Fatalf("unexpected responseTime: %v", out["responseTime"])
	}

	if out["timestamp"] != "2026-01-05T15:04:05Z" {
		t.Fatalf("unexpected timestamp: %v", out["timestamp"])
	}
}
