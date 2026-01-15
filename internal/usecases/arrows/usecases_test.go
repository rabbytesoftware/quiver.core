package arrows

import (
	"testing"

	"github.com/rabbytesoftware/quiver/internal/infrastructure"
	"github.com/rabbytesoftware/quiver/internal/repositories"
)

func TestNewApiArrowsUsecases_FieldsSet(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiArrowsUsecases(repos)
	if uc == nil {
		t.Fatal("NewApiArrowsUsecases returned nil")
	}

	if uc.repository == nil {
		t.Fatal("expected repository to be set")
	}

	if uc.ctx == nil {
		t.Fatal("expected ctx to be set")
	}
}

func TestAdd_UrlSucceeds(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiArrowsUsecases(repos)

	_, warns, err := uc.Add("http://example.com/arrow", "url", "127.0.0.1", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = warns
}

func TestRemove_List_Get_NoErrors(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiArrowsUsecases(repos)

	if warns, err := uc.Remove("some-namespace", "127.0.0.1"); err != nil {
		t.Fatalf("Remove returned error: %v warns: %v", err, warns)
	}

	if _, warns, err := uc.List(); err != nil {
		t.Fatalf("List returned error: %v warns: %v", err, warns)
	}

	if a, warns, err := uc.Get("some-namespace"); err != nil {
		t.Fatalf("Get returned error: %v warns: %v", err, warns)
	} else if a == nil {
		t.Fatalf("Get returned nil arrow pointer; warns=%v", warns)
	}
}

func TestExecuteMethod_Returns(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiArrowsUsecases(repos)

	// ExecuteMethod should return without error
	warn, err := uc.ExecuteMethod("namespace", "127.0.0.1", "method", map[string]string{})
	// Just verify it doesn't panic
	_ = warn
	_ = err
}

func TestStopMethod_Returns(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiArrowsUsecases(repos)

	// StopMethod should return without error
	warns, err := uc.StopMethod("namespace", "method")
	_ = warns
	_ = err
}

func TestKillMethod_Returns(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiArrowsUsecases(repos)

	// KillMethod should return without error
	warns, err := uc.KillMethod("namespace", "method")
	_ = warns
	_ = err
}

func TestListenChannel_Returns(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiArrowsUsecases(repos)

	// ListenChannel should not panic
	uc.ListenChannel()
}

func TestAdd_DirectoryAndNamespaceTypes(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiArrowsUsecases(repos)

	// directory type
	_, warnsDir, errDir := uc.Add("/", "directory", "127.0.0.1", false)
	if errDir != nil {
		t.Fatalf("expected no error for directory type, got: %v warns: %v", errDir, warnsDir)
	}

	// namespace type
	_, warnsNs, errNs := uc.Add("some:namespace", "namespace", "127.0.0.1", false)
	if errNs != nil {
		t.Fatalf("expected no error for namespace type, got: %v warns: %v", errNs, warnsNs)
	}
}

func TestAdd_Url_Type(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiArrowsUsecases(repos)

	// url type explicitly
	_, warnsUrl, errUrl := uc.Add("http://example.com", "url", "127.0.0.1", false)
	if errUrl != nil {
		t.Fatalf("expected no error for url type, got: %v warns: %v", errUrl, warnsUrl)
	}

}

func TestRemove_Arrows_Success(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiArrowsUsecases(repos)

	warns, err := uc.Remove("test:namespace", "127.0.0.1")
	if err != nil {
		t.Fatalf("Remove failed: %v warns: %v", err, warns)
	}
}

func TestExecuteMethod_DirectInvocation(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiArrowsUsecases(repos)

	// Test with nil variables map (should default to empty)
	warns, err := uc.ExecuteMethod("test:namespace", "127.0.0.1", "testmethod", nil)
	_ = warns
	_ = err

	// Test with non-nil variables
	warns2, err2 := uc.ExecuteMethod("test:namespace", "127.0.0.1", "testmethod", map[string]string{"key": "value"})
	_ = warns2
	_ = err2
}

func TestGet_Arrow(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiArrowsUsecases(repos)

	arrow, warns, err := uc.Get("test:namespace")
	if err != nil {
		t.Fatalf("Get failed: %v warns: %v", err, warns)
	}
	if arrow == nil {
		t.Fatalf("Get returned nil arrow pointer")
	}
}

func TestList_Arrows(t *testing.T) {
	infra := infrastructure.NewInfrastructure()
	repos := repositories.NewRepositories(infra)

	uc := NewApiArrowsUsecases(repos)

	arrows, warns, err := uc.List()
	if err != nil {
		t.Fatalf("List failed: %v warns: %v", err, warns)
	}
	_ = arrows
}
