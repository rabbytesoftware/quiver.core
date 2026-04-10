package middleware

import (
	"net/http"
	"testing"
)

// TestUpgraderExportedAndConfigured verifies the Upgrader is exported and properly configured.
func TestUpgraderExportedAndConfigured(t *testing.T) {
	if Upgrader.CheckOrigin == nil {
		t.Fatal("Upgrader.CheckOrigin must not be nil")
	}

	// Verify CheckOrigin accepts requests (v0 — no auth).
	req := &http.Request{}
	if !Upgrader.CheckOrigin(req) {
		t.Fatal("Upgrader.CheckOrigin should accept all origins")
	}
}
