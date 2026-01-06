package docs

import (
	"strings"
	"testing"

	"github.com/swaggo/swag"
)

func TestSwaggerInfo_Basics(t *testing.T) {
	if SwaggerInfo == nil {
		t.Fatal("SwaggerInfo is nil")
	}

	if SwaggerInfo.InfoInstanceName != "swagger" {
		t.Fatalf("unexpected InfoInstanceName: %s", SwaggerInfo.InfoInstanceName)
	}

	if SwaggerInfo.LeftDelim != "{{" || SwaggerInfo.RightDelim != "}}" {
		t.Fatalf("unexpected template delimiters: %q %q", SwaggerInfo.LeftDelim, SwaggerInfo.RightDelim)
	}

	if !strings.Contains(SwaggerInfo.SwaggerTemplate, "\"swagger\": \"2.0\"") {
		t.Fatal("docTemplate does not contain swagger version")
	}
}

func TestDocTemplate_ContainsPaths(t *testing.T) {
	if !strings.Contains(docTemplate, "/api/v1/arrow") {
		t.Fatal("docTemplate missing expected path /api/v1/arrow")
	}

	if !strings.Contains(docTemplate, "Execute arrow method") {
		t.Fatal("docTemplate missing expected summary 'Execute arrow method'")
	}
}

func TestSwagRegister_ReadDoc(t *testing.T) {
	// Ensure that swag.ReadDoc returns a non-empty string for the registered spec
	name := SwaggerInfo.InstanceName()
	doc, err := swag.ReadDoc(name)

	if err != nil {
		t.Fatalf("swag.ReadDoc returned error for instance %s", name)
	}

	if doc == "" {
		t.Fatalf("swag.ReadDoc returned empty for instance %s", name)
	}

	if !strings.Contains(doc, "\"swagger\": \"2.0\"") {
		t.Fatal("returned doc does not contain swagger version")
	}
}
