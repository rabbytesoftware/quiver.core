package manifold

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/assembler"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator"
)

type stubResolver struct {
	arrowData  []byte
	arrowErr   error
	quiverData []byte
	quiverErr  error
}

func (s *stubResolver) ResolveArrow(
	_ context.Context,
	_ domain.Namespace,
) ([]byte, error) {
	return s.arrowData, s.arrowErr
}

func (s *stubResolver) ResolveQuiver(
	_ context.Context,
	_ domain.Namespace,
) ([]byte, error) {
	return s.quiverData, s.quiverErr
}

type stubTranslator struct {
	arrowErr  error
	arrow     *domain.ArrowManifest
	quiverErr error
	quiver    *domain.QuiverManifest
}

func (s *stubTranslator) Arrow(data []byte) (*domain.ArrowManifest, error) {
	return s.arrow, s.arrowErr
}

func (s *stubTranslator) Quiver(data []byte) (*domain.QuiverManifest, error) {
	return s.quiver, s.quiverErr
}

func (s *stubTranslator) ReadSchemaInfo(data []byte) (*translator.ManifestInfo, error) {
	return nil, nil
}

func TestNew_ReturnsManifoldInterface(t *testing.T) {
	var _ Manifold = New(0)
}

func TestNew_CustomTimeout(t *testing.T) {
	var _ Manifold = New(10 * time.Second)
}

func TestResolveArrow_InvalidNamespace(t *testing.T) {
	namespaceErr := errors.New("invalid namespace")
	m := &manifold{
		rsv: &stubResolver{arrowErr: namespaceErr},
		trs: &stubTranslator{},
	}
	_, err := m.ResolveArrow(context.Background(), domain.Namespace("not-valid"))
	if !errors.Is(err, namespaceErr) {
		t.Fatalf("expected namespace error, got %v", err)
	}
}

func TestResolveArrow_UnsupportedPlatform(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{arrowErr: resolver.ErrUnsupportedPlatform},
		trs: &stubTranslator{},
	}
	_, err := m.ResolveArrow(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if !errors.Is(err, resolver.ErrUnsupportedPlatform) {
		t.Errorf("expected ErrUnsupportedPlatform, got %v", err)
	}
}

func TestResolveArrow_ResolverError(t *testing.T) {
	resolveErr := errors.New("resolver failed")
	m := &manifold{
		rsv: &stubResolver{arrowErr: resolveErr},
		trs: &stubTranslator{},
	}
	_, err := m.ResolveArrow(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if !errors.Is(err, resolveErr) {
		t.Errorf("expected resolveErr, got %v", err)
	}
}

func TestResolveArrow_TranslatorError(t *testing.T) {
	translateErr := errors.New("translator failed")
	m := &manifold{
		rsv: &stubResolver{arrowData: []byte("test")},
		trs: &stubTranslator{arrowErr: translateErr},
	}
	_, err := m.ResolveArrow(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if !errors.Is(err, translateErr) {
		t.Errorf("expected translateErr, got %v", err)
	}
}

func TestResolveArrow_Success(t *testing.T) {
	expectedManifest := &domain.ArrowManifest{
		Name:    "my-arrow",
		Version: "1.0.0",
	}
	m := &manifold{
		rsv: &stubResolver{arrowData: []byte("test")},
		trs: &stubTranslator{arrow: expectedManifest},
	}
	result, err := m.ResolveArrow(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "my-arrow" {
		t.Errorf("Name = %q, want my-arrow", result.Name)
	}
}

func TestResolveQuiver_InvalidNamespace(t *testing.T) {
	namespaceErr := errors.New("invalid namespace")
	m := &manifold{
		rsv: &stubResolver{quiverErr: namespaceErr},
		trs: &stubTranslator{},
	}
	_, err := m.ResolveQuiver(context.Background(), domain.Namespace("not-valid"))
	if !errors.Is(err, namespaceErr) {
		t.Fatalf("expected namespace error, got %v", err)
	}
}

func TestResolveQuiver_UnsupportedPlatform(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{quiverErr: resolver.ErrUnsupportedPlatform},
		trs: &stubTranslator{},
	}
	_, err := m.ResolveQuiver(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if !errors.Is(err, resolver.ErrUnsupportedPlatform) {
		t.Errorf("expected ErrUnsupportedPlatform, got %v", err)
	}
}

func TestResolveQuiver_ResolverError(t *testing.T) {
	resolveErr := errors.New("resolver failed")
	m := &manifold{
		rsv: &stubResolver{quiverErr: resolveErr},
		trs: &stubTranslator{},
	}
	_, err := m.ResolveQuiver(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if !errors.Is(err, resolveErr) {
		t.Errorf("expected resolveErr, got %v", err)
	}
}

func TestResolveQuiver_TranslatorError(t *testing.T) {
	translateErr := errors.New("translator failed")
	m := &manifold{
		rsv: &stubResolver{quiverData: []byte("test")},
		trs: &stubTranslator{quiverErr: translateErr},
	}
	_, err := m.ResolveQuiver(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if !errors.Is(err, translateErr) {
		t.Errorf("expected translateErr, got %v", err)
	}
}

func TestResolveQuiver_Success(t *testing.T) {
	expectedManifest := &domain.QuiverManifest{
		Name:        "my-quiver",
		Description: "A test quiver",
	}
	m := &manifold{
		rsv: &stubResolver{quiverData: []byte("test")},
		trs: &stubTranslator{quiver: expectedManifest},
	}
	result, err := m.ResolveQuiver(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "my-quiver" {
		t.Errorf("Name = %q, want my-quiver", result.Name)
	}
}

func TestParseArrow_TranslatorError(t *testing.T) {
	translateErr := errors.New("bad yaml")
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{arrowErr: translateErr},
	}
	_, err := m.ParseArrow([]byte("bad yaml"))
	if !errors.Is(err, translateErr) {
		t.Fatalf("expected translateErr, got %v", err)
	}
}

func TestParseArrow_AssemblerError_ReturnsStructuredErrors(t *testing.T) {
	invalidManifest := &domain.ArrowManifest{
		Name:    "test",
		Version: "1.0.0",
		Lifecycle: domain.Lifecycle{
			Install: step.StepList{
				step.NewRunStep("Install", "install.sh", 0, true),
			},
			// missing uninstall — assembler will catch this
		},
	}
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{arrow: invalidManifest},
	}
	_, err := m.ParseArrow([]byte("any"))
	if err == nil {
		t.Fatal("expected assembler error")
	}
	var asmErrs assembler.AssemblerErrors
	if !errors.As(err, &asmErrs) {
		t.Fatalf("expected AssemblerErrors, got %T: %v", err, err)
	}
}

func TestParseArrow_ValidManifest_ReturnsManifest(t *testing.T) {
	validManifest := &domain.ArrowManifest{
		Name:    "my-arrow",
		Version: "1.0.0",
	}
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{arrow: validManifest},
	}
	result, err := m.ParseArrow([]byte("any"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "my-arrow" {
		t.Errorf("Name = %q, want my-arrow", result.Name)
	}
}

func TestResolveArrow_AssemblerValidationError(t *testing.T) {
	// ArrowManifest with duplicate variables will fail validation
	invalidManifest := &domain.ArrowManifest{
		Name:    "test",
		Version: "1.0.0",
		Variables: []domain.Variable{
			{Name: "VAR1"},
			{Name: "VAR1"}, // duplicate
		},
		Lifecycle: domain.Lifecycle{
			Install: step.StepList{
				step.NewRunStep("Install", "install.sh", 0, true),
			},
			Uninstall: step.StepList{
				step.NewRunStep("Uninstall", "uninstall.sh", 0, true),
			},
		},
	}
	m := &manifold{
		rsv: &stubResolver{arrowData: []byte("test")},
		trs: &stubTranslator{arrow: invalidManifest},
	}
	_, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/user/repo"))
	if err == nil {
		t.Fatal("expected error for invalid arrow manifest")
	}
}
