package arrows

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestExecuteMethodRequestDTO_MarshalUnmarshal(t *testing.T) {
	dto := ExecuteMethodRequestDTO{
		Variables: map[string]string{"key": "value", "foo": "bar"},
	}

	b, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got ExecuteMethodRequestDTO
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if !reflect.DeepEqual(dto.Variables, got.Variables) {
		t.Fatalf("variables mismatch: got=%v want=%v", got.Variables, dto.Variables)
	}
}

func TestExecuteMethodRequestDTO_UnmarshalEmptyJSON(t *testing.T) {
	var got ExecuteMethodRequestDTO
	if err := json.Unmarshal([]byte(`{}`), &got); err != nil {
		t.Fatalf("unmarshal empty json failed: %v", err)
	}

	if got.Variables != nil {
		t.Fatalf("expected Variables to be nil for empty json, got: %v", got.Variables)
	}
}
