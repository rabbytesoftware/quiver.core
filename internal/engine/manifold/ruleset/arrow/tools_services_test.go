package arrow

import (
	"testing"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestToolsServicesRule_Valid(t *testing.T) {
	rule := ToolsServicesRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Tools:    []domain.DependencyEdge{{Namespace: "github.com/user/repo/tool", Type: domain.ToolDep}},
				Services: []domain.DependencyEdge{{Namespace: "github.com/user/other/service", Type: domain.ServiceDep}},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors, got: %v", errs)
	}
}

func TestToolsServicesRule_Overlap(t *testing.T) {
	rule := ToolsServicesRule{}
	ns := domain.Namespace("github.com/user/repo/tool")
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Tools:    []domain.DependencyEdge{{Namespace: ns, Type: domain.ToolDep}},
				Services: []domain.DependencyEdge{{Namespace: ns, Type: domain.ServiceDep}},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) == 0 {
		t.Fatal("expected errors for namespace appearing in both tools and services, got none")
	}
	if errs[0].Rule != "tools_services_overlap" {
		t.Fatalf("expected rule %q, got %q", "tools_services_overlap", errs[0].Rule)
	}
}

func TestToolsServicesRule_OnlyTools(t *testing.T) {
	rule := ToolsServicesRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Tools: []domain.DependencyEdge{{Namespace: "github.com/user/repo/tool", Type: domain.ToolDep}},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for only tools, got: %v", errs)
	}
}

func TestToolsServicesRule_OnlyServices(t *testing.T) {
	rule := ToolsServicesRule{}
	m := &domain.Arrow{
		Targets: map[domain.OS]domain.Target{
			domain.OSLinuxAMD64: {
				Services: []domain.DependencyEdge{{Namespace: "github.com/user/repo/service", Type: domain.ServiceDep}},
			},
		},
	}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for only services, got: %v", errs)
	}
}

func TestToolsServicesRule_EmptyTargets(t *testing.T) {
	rule := ToolsServicesRule{}
	m := &domain.Arrow{}
	errs := rule.Validate(m)
	if len(errs) != 0 {
		t.Fatalf("expected no errors for empty targets, got: %v", errs)
	}
}
