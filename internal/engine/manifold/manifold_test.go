package manifold

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/compiler"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/resolver"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/ruleset"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/translator"
	v0 "github.com/rabbytesoftware/quiver/internal/engine/manifold/translator/arrow/v0"
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
) ([]byte, string, error) {
	return s.arrowData, "", s.arrowErr
}

func (s *stubResolver) ResolveQuiver(
	_ context.Context,
	_ domain.Namespace,
) ([]byte, error) {
	return s.quiverData, s.quiverErr
}

type stubTranslator struct {
	arrowErr    error
	arrow       *domain.Arrow
	precompiled map[string]models.PrecompiledTarget
	quiverErr   error
	quiver      *domain.QuiverManifest
}

func (s *stubTranslator) Arrow(data []byte) (translator.Module, error) {
	if s.arrowErr != nil {
		return translator.Module{}, s.arrowErr
	}
	return translator.Module{
		Manifest:    s.arrow,
		Precompiled: s.precompiled,
		Selector:    v0.New().Selector(),
	}, nil
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
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("not-valid"))
	if !errors.Is(err, namespaceErr) {
		t.Fatalf("expected namespace error, got %v", err)
	}
}

func TestResolveArrow_UnsupportedPlatform(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{arrowErr: resolver.ErrUnsupportedPlatform},
		trs: &stubTranslator{},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, _, _, err := m.ResolveArrow(
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
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, _, _, err := m.ResolveArrow(
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
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, _, _, err := m.ResolveArrow(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if !errors.Is(err, translateErr) {
		t.Errorf("expected translateErr, got %v", err)
	}
}

func TestResolveArrow_Success(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{
					step.NewRunStep("install", "echo ok", false, "10s", true),
				},
				Uninstall: step.StepList{
					step.NewRunStep("uninstall", "echo bye", false, "10s", true),
				},
			},
		},
	}
	expectedManifest := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:    "my-arrow",
			Version: "1.0.0",
		},
	}
	m := &manifold{
		rsv: &stubResolver{arrowData: []byte("test")},
		trs: &stubTranslator{arrow: expectedManifest, precompiled: precompiled},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	result, _, _, err := m.ResolveArrow(
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
		cmp: compiler.New(),
		rls: ruleset.New(),
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
		cmp: compiler.New(),
		rls: ruleset.New(),
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
		cmp: compiler.New(),
		rls: ruleset.New(),
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
		cmp: compiler.New(),
		rls: ruleset.New(),
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
		cmp: compiler.New(),
		rls: ruleset.New(),
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
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, err := m.ParseArrow([]byte("bad yaml"))
	if !errors.Is(err, translateErr) {
		t.Fatalf("expected translateErr, got %v", err)
	}
}

func TestParseArrow_RuleError_ReturnsStructuredErrors(t *testing.T) {
	// A manifest where the compiled targets have install without uninstall → ruleset error.
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{
					step.NewRunStep("Install", "install.sh", false, "", true),
				},
				// missing uninstall — ruleset will catch this
			},
		},
	}
	invalidManifest := &domain.Arrow{}
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{arrow: invalidManifest, precompiled: precompiled},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, err := m.ParseArrow([]byte("any"))
	if err == nil {
		t.Fatal("expected assembler error")
	}
	var asmErrs ruleset.RuleErrors
	if !errors.As(err, &asmErrs) {
		t.Fatalf("expected RuleErrors, got %T: %v", err, err)
	}
}

func TestParseArrow_ValidManifest_ReturnsManifest(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{
					step.NewRunStep("install", "echo ok", false, "10s", true),
				},
				Uninstall: step.StepList{
					step.NewRunStep("uninstall", "echo bye", false, "10s", true),
				},
			},
		},
	}
	validManifest := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:    "my-arrow",
			Version: "1.0.0",
		},
	}
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{arrow: validManifest, precompiled: precompiled},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	result, err := m.ParseArrow([]byte("any"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "my-arrow" {
		t.Errorf("Name = %q, want my-arrow", result.Name)
	}
}

func TestParseArrow_PostCompileValidationError(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{
					step.NewRunStep("install", "echo ok", false, "10s", true),
				},
				Uninstall: step.StepList{
					step.NewRunStep("uninstall", "echo bye", false, "10s", true),
				},
			},
		},
	}
	validManifest := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:    "my-arrow",
			Version: "1.0.0",
		},
	}
	stubRuleset := &stubRuleset{postCompileErr: errors.New("post-compile validation failed")}
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{arrow: validManifest, precompiled: precompiled},
		cmp: compiler.New(),
		rls: stubRuleset,
	}
	_, err := m.ParseArrow([]byte("any"))
	if err == nil {
		t.Fatal("expected error for post-compile validation failure")
	}
}

type stubRuleset struct {
	precompileErr  error
	postCompileErr error
}

func (s *stubRuleset) ValidatePrecompile(m *domain.Arrow, p map[string]models.PrecompiledTarget) error {
	return s.precompileErr
}

func (s *stubRuleset) ValidateCompiled(m *domain.Arrow) error {
	return s.postCompileErr
}

func TestResolveArrow_AssemblerValidationError(t *testing.T) {
	// ArrowManifest with duplicate variables will fail validation
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{
					step.NewRunStep("Install", "install.sh", false, "", true),
				},
				Uninstall: step.StepList{
					step.NewRunStep("Uninstall", "uninstall.sh", false, "", true),
				},
			},
		},
	}
	invalidManifest := &domain.Arrow{
		Variables: []domain.Variable{
			{Name: "VAR1"},
			{Name: "VAR1"}, // duplicate
		},
	}
	m := &manifold{
		rsv: &stubResolver{arrowData: []byte("test")},
		trs: &stubTranslator{arrow: invalidManifest, precompiled: precompiled},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/user/repo"))
	if err == nil {
		t.Fatal("expected error for invalid arrow manifest")
	}
}

func TestParseArrow_CompileError(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install: step.StepList{
					step.NewRunStep("install", "echo ok", false, "10s", true),
				},
				Uninstall: step.StepList{
					step.NewRunStep("uninstall", "echo bye", false, "10s", true),
				},
			},
		},
	}
	validManifest := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{
			Name:    "my-arrow",
			Version: "1.0.0",
		},
	}
	compileErr := errors.New("compile failed")
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{arrow: validManifest, precompiled: precompiled},
		cmp: &stubCompiler{err: compileErr},
		rls: ruleset.New(),
	}
	_, err := m.ParseArrow([]byte("any"))
	if !errors.Is(err, compileErr) && err == nil {
		t.Fatalf("expected compile error, got %v", err)
	}
}

func TestResolveQuiver_SuccessAfterResolve(t *testing.T) {
	expectedManifest := &domain.QuiverManifest{
		Name:        "test-quiver",
		Description: "A test quiver",
	}
	m := &manifold{
		rsv: &stubResolver{quiverData: []byte("test")},
		trs: &stubTranslator{quiver: expectedManifest},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	result, err := m.ResolveQuiver(context.Background(), domain.Namespace("github.com/user/repo"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "test-quiver" {
		t.Errorf("Name = %q, want test-quiver", result.Name)
	}
}

type stubCompiler struct {
	err error
}

func (s *stubCompiler) Compile(_ *domain.Arrow, _ map[string]models.PrecompiledTarget, _ models.Selector) error {
	return s.err
}
