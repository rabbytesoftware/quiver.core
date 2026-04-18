package rules

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/netbridge"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
)

func TestNetbridgeRule_Valid(t *testing.T) {
	rule := NetbridgeRule{}
	m := &domain.ArrowManifest{
		Netbridge: []netbridge.PortDef{
			{Name: "GAME_PORT", Protocol: "tcp", Default: 27015},
			{Name: "API_PORT", Protocol: "udp", Default: 8080},
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestNetbridgeRule_InvalidPort_EmptyName(t *testing.T) {
	rule := NetbridgeRule{}
	m := &domain.ArrowManifest{
		Netbridge: []netbridge.PortDef{
			{Name: "", Protocol: "tcp", Default: 27015},
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) == 0 {
		t.Fatal("expected errors for empty port name, got none")
	}
	if errs[0].Rule != "invalid_port" {
		t.Fatalf("expected rule %q, got %q", "invalid_port", errs[0].Rule)
	}
}

func TestNetbridgeRule_InvalidPort_BadProtocol(t *testing.T) {
	rule := NetbridgeRule{}
	m := &domain.ArrowManifest{
		Netbridge: []netbridge.PortDef{
			{Name: "MY_PORT", Protocol: "ftp", Default: 27015},
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) == 0 {
		t.Fatal("expected errors for invalid protocol, got none")
	}
	if errs[0].Rule != "invalid_port" {
		t.Fatalf("expected rule %q, got %q", "invalid_port", errs[0].Rule)
	}
}

func TestNetbridgeRule_DuplicateName(t *testing.T) {
	rule := NetbridgeRule{}
	m := &domain.ArrowManifest{
		Netbridge: []netbridge.PortDef{
			{Name: "MY_PORT", Protocol: "tcp", Default: 27015},
			{Name: "MY_PORT", Protocol: "udp", Default: 8080},
		},
	}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) == 0 {
		t.Fatal("expected errors for duplicate port name, got none")
	}
	found := false
	for _, e := range errs {
		if e.Rule == "duplicate_name" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected rule %q in errors, got: %v", "duplicate_name", errs)
	}
}

func TestNetbridgeRule_NoEntries(t *testing.T) {
	rule := NetbridgeRule{}
	m := &domain.ArrowManifest{}
	errs := rule.Validate(m, map[string]models.PrecompiledTarget{})
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty netbridge, got: %v", errs)
	}
}
