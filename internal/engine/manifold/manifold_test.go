package manifold

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/compiler"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
	resolvers "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver/resolvers"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/ruleset"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator"
	v0 "github.com/rabbytesoftware/quiver.core/internal/engine/manifold/translator/arrow/v0"
)

type stubResolver struct {
	arrowData     []byte
	arrowFilename string
	arrowErr      error
	quiverData    []byte
	quiverErr     error
}

func (s *stubResolver) ResolveArrow(
	_ context.Context,
	_ domain.Namespace,
) ([]byte, string, error) {
	return s.arrowData, s.arrowFilename, s.arrowErr
}

func (s *stubResolver) ResolveCollection(
	_ context.Context,
	_ domain.Namespace,
) ([]byte, error) {
	return s.quiverData, s.quiverErr
}

type stubTranslator struct {
	arrowErr      error
	arrow         *domain.Arrow
	precompiled   map[string]models.PrecompiledTarget
	quiverErr     error
	quiver        *domain.Collection
	quiverEntries []domain.CollectionArrowEntry
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

func (s *stubTranslator) Collection(data []byte) (translator.CollectionModule, error) {
	if s.quiverErr != nil {
		return translator.CollectionModule{}, s.quiverErr
	}
	var manifest domain.Collection
	if s.quiver != nil {
		manifest = *s.quiver
	}
	return translator.CollectionModule{Manifest: manifest, Entries: s.quiverEntries}, nil
}

func (s *stubTranslator) ReadSchemaInfo(data []byte) (*translator.ManifestInfo, error) {
	return nil, nil
}

func TestNew_ReturnsManifoldInterface(t *testing.T) {
	_ = New(0)
}

func TestNew_CustomTimeout(t *testing.T) {
	_ = New(10 * time.Second)
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
		rsv: &stubResolver{arrowData: []byte("test"), arrowFilename: "ARROW.md"},
		trs: &stubTranslator{arrow: expectedManifest, precompiled: precompiled},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	result, _, filename, err := m.ResolveArrow(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "my-arrow" {
		t.Errorf("Name = %q, want my-arrow", result.Name)
	}
	if filename != "ARROW.md" {
		t.Errorf("filename = %q, want ARROW.md", filename)
	}
}

func TestResolveCollection_InvalidNamespace(t *testing.T) {
	namespaceErr := errors.New("invalid namespace")
	m := &manifold{
		rsv: &stubResolver{quiverErr: namespaceErr},
		trs: &stubTranslator{},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, err := m.ResolveCollection(context.Background(), domain.Namespace("not-valid"))
	if !errors.Is(err, namespaceErr) {
		t.Fatalf("expected namespace error, got %v", err)
	}
}

func TestResolveCollection_UnsupportedPlatform(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{quiverErr: resolver.ErrUnsupportedPlatform},
		trs: &stubTranslator{},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, err := m.ResolveCollection(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if !errors.Is(err, resolver.ErrUnsupportedPlatform) {
		t.Errorf("expected ErrUnsupportedPlatform, got %v", err)
	}
}

func TestResolveCollection_ResolverError(t *testing.T) {
	resolveErr := errors.New("resolver failed")
	m := &manifold{
		rsv: &stubResolver{quiverErr: resolveErr},
		trs: &stubTranslator{},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, err := m.ResolveCollection(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if !errors.Is(err, resolveErr) {
		t.Errorf("expected resolveErr, got %v", err)
	}
}

func TestResolveCollection_TranslatorError(t *testing.T) {
	translateErr := errors.New("translator failed")
	m := &manifold{
		rsv: &stubResolver{quiverData: []byte("test")},
		trs: &stubTranslator{quiverErr: translateErr},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, err := m.ResolveCollection(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if !errors.Is(err, translateErr) {
		t.Errorf("expected translateErr, got %v", err)
	}
}

func TestResolveCollection_Success(t *testing.T) {
	expectedManifest := &domain.Collection{
		Meta: domain.CollectionMeta{Name: "my-quiver", Description: "A test quiver"},
	}
	m := &manifold{
		rsv: &stubResolver{quiverData: []byte("test")},
		trs: &stubTranslator{
			quiver:        expectedManifest,
			quiverEntries: []domain.CollectionArrowEntry{{Namespace: "github.com/user/tool"}},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	result, err := m.ResolveCollection(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Meta.Name != "my-quiver" {
		t.Errorf("Name = %q, want my-quiver", result.Meta.Name)
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
	quiverErr      error
}

func (s *stubRuleset) ValidatePrecompile(m *domain.Arrow, p map[string]models.PrecompiledTarget) error {
	return s.precompileErr
}

func (s *stubRuleset) ValidateCompiled(m *domain.Arrow) error {
	return s.postCompileErr
}

func (s *stubRuleset) ValidateCollection(m *domain.Collection) error {
	return s.quiverErr
}

func (s *stubRuleset) ValidateCollectionEntries(entries []domain.CollectionArrowEntry) error {
	return nil
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

func TestResolveCollection_SuccessAfterResolve(t *testing.T) {
	expectedManifest := &domain.Collection{
		Meta: domain.CollectionMeta{Name: "test-quiver", Description: "A test quiver"},
	}
	m := &manifold{
		rsv: &stubResolver{quiverData: []byte("test")},
		trs: &stubTranslator{
			quiver:        expectedManifest,
			quiverEntries: []domain.CollectionArrowEntry{{Namespace: "github.com/user/tool"}},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	result, err := m.ResolveCollection(context.Background(), domain.Namespace("github.com/user/repo"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Meta.Name != "test-quiver" {
		t.Errorf("Name = %q, want test-quiver", result.Meta.Name)
	}
}

type stubCompiler struct {
	err error
}

func (s *stubCompiler) Compile(_ *domain.Arrow, _ map[string]models.PrecompiledTarget, _ models.Selector) error {
	return s.err
}

type stubConstraintResolver struct {
	result string
	err    error
}

func (s *stubConstraintResolver) Resolve(_ context.Context, _ domain.Namespace, _ string) (string, error) {
	return s.result, s.err
}

func TestNewWithResolvers_ReturnsManifoldInterface(t *testing.T) {
	rsv := &stubResolver{}
	crs := &stubConstraintResolver{}
	_ = NewWithResolvers(rsv, crs)
}

func TestNewWithResolvers_UsesInjectedResolver(t *testing.T) {
	resolveErr := errors.New("injected resolver failed")
	rsv := &stubResolver{arrowErr: resolveErr}
	crs := &stubConstraintResolver{}

	m := NewWithResolvers(rsv, crs)
	_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/user/repo"))

	if !errors.Is(err, resolveErr) {
		t.Fatalf("expected injected resolver error, got %v", err)
	}
}

func TestResolveConstraint_Success(t *testing.T) {
	rsv := &stubResolver{}
	crs := &stubConstraintResolver{result: "v1.2.3"}

	m := NewWithResolvers(rsv, crs)
	got, err := m.ResolveConstraint(context.Background(), domain.Namespace("github.com/user/repo@v1.*"), "v1.*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.2.3" {
		t.Errorf("expected v1.2.3, got %q", got)
	}
}

func TestResolveConstraint_Error(t *testing.T) {
	constraintErr := errors.New("no matching tags")
	rsv := &stubResolver{}
	crs := &stubConstraintResolver{err: constraintErr}

	m := NewWithResolvers(rsv, crs)
	_, err := m.ResolveConstraint(context.Background(), domain.Namespace("github.com/user/repo@v1.*"), "v1.*")

	if !errors.Is(err, constraintErr) {
		t.Fatalf("expected constraintErr, got %v", err)
	}
}

func TestNewWithResolvers_ConstraintResolver_UsedOnResolveConstraint(t *testing.T) {
	_ = resolvers.NewConstraintResolver(0)

	rsv := &stubResolver{}
	crs := &stubConstraintResolver{result: "v2.0.0"}

	m := NewWithResolvers(rsv, crs)
	got, err := m.ResolveConstraint(context.Background(), domain.Namespace("github.com/org/pkg@v2.*"), "v2.*")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v2.0.0" {
		t.Errorf("expected v2.0.0, got %q", got)
	}
}

func TestParseCollection_DeriveLocalArrowNamespace(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Name: "Gaming", Description: "desc"},
			},
			quiverEntries: []domain.CollectionArrowEntry{
				{Path: "servers/cs2"},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	ns := domain.Namespace("github.com/char2cs/gaming.quiver")
	manifest, err := m.ParseCollection([]byte("any"), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifest.Arrows) != 1 {
		t.Fatalf("expected 1 arrow, got %d", len(manifest.Arrows))
	}
	want := domain.Namespace("github.com/char2cs/gaming.quiver/cs2")
	if manifest.Arrows[0].Namespace != want {
		t.Errorf("Namespace = %q, want %q", manifest.Arrows[0].Namespace, want)
	}
}

func TestParseCollection_ExternalArrowPassthrough(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Name: "Gaming", Description: "desc"},
			},
			quiverEntries: []domain.CollectionArrowEntry{
				{Namespace: "github.com/other/pkg"},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	manifest, err := m.ParseCollection([]byte("any"), domain.Namespace("github.com/char2cs/gaming.quiver"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifest.Arrows) != 1 {
		t.Fatalf("expected 1 arrow, got %d", len(manifest.Arrows))
	}
	want := domain.Namespace("github.com/other/pkg")
	if manifest.Arrows[0].Namespace != want {
		t.Errorf("Namespace = %q, want %q", manifest.Arrows[0].Namespace, want)
	}
}

func TestParseCollection_MixedLocalAndExternal(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Name: "Gaming", Description: "desc"},
			},
			quiverEntries: []domain.CollectionArrowEntry{
				{Path: "servers/cs2"},
				{Namespace: "github.com/other/pkg"},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	manifest, err := m.ParseCollection([]byte("any"), domain.Namespace("github.com/char2cs/gaming.quiver"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(manifest.Arrows) != 2 {
		t.Fatalf("expected 2 arrows, got %d", len(manifest.Arrows))
	}
	if manifest.Arrows[0].Namespace != "github.com/char2cs/gaming.quiver/cs2" {
		t.Errorf("arrow[0] Namespace = %q, want github.com/char2cs/gaming.quiver/cs2", manifest.Arrows[0].Namespace)
	}
	if manifest.Arrows[1].Namespace != "github.com/other/pkg" {
		t.Errorf("arrow[1] Namespace = %q, want github.com/other/pkg", manifest.Arrows[1].Namespace)
	}
}

func TestParseCollection_ValidateCollectionError_MissingName(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Description: "desc"},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, err := m.ParseCollection([]byte("any"), domain.Namespace("github.com/char2cs/gaming.quiver"))
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestParseCollection_ValidateCollectionError_DuplicateArrows(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Name: "Gaming", Description: "desc"},
			},
			quiverEntries: []domain.CollectionArrowEntry{
				{Path: "arrows/cs2"},
				{Path: "other/cs2"},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, err := m.ParseCollection([]byte("any"), domain.Namespace("github.com/char2cs/gaming.quiver"))
	if err == nil {
		t.Fatal("expected error for duplicate arrow namespaces")
	}
}

func TestParseCollection_BothPathAndNamespaceSet_ReturnsError(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Name: "Gaming", Description: "desc"},
			},
			quiverEntries: []domain.CollectionArrowEntry{
				{Path: "servers/cs2", Namespace: "github.com/other/pkg"},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, err := m.ParseCollection([]byte("any"), domain.Namespace("github.com/char2cs/gaming.quiver"))
	if err == nil {
		t.Fatal("expected error when both path and namespace are set")
	}
}

func TestParseCollection_TranslatorError(t *testing.T) {
	translateErr := errors.New("quiver translator failed")
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{quiverErr: translateErr},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, err := m.ParseCollection([]byte("bad"), domain.Namespace("github.com/char2cs/gaming.quiver"))
	if !errors.Is(err, translateErr) {
		t.Fatalf("expected translateErr, got %v", err)
	}
}

func TestParseCollection_VersionedNamespace_StripsRef(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Name: "Gaming", Description: "desc"},
			},
			quiverEntries: []domain.CollectionArrowEntry{
				{Path: "servers/cs2"},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	ns := domain.Namespace("github.com/char2cs/gaming.quiver@v1.2.3")
	manifest, err := m.ParseCollection([]byte("any"), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := domain.Namespace("github.com/char2cs/gaming.quiver/cs2")
	if manifest.Arrows[0].Namespace != want {
		t.Errorf("Namespace = %q, want %q", manifest.Arrows[0].Namespace, want)
	}
}

func TestParseCollection_NeitherPathNorNamespace_ReturnsError(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Name: "Gaming", Description: "desc"},
			},
			quiverEntries: []domain.CollectionArrowEntry{
				{},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, err := m.ParseCollection([]byte("any"), domain.Namespace("github.com/char2cs/gaming.quiver"))
	if err == nil {
		t.Fatal("expected error when neither path nor namespace are set")
	}
}

func TestParseCollection_IsLocal_SetCorrectly(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Name: "Gaming", Description: "desc"},
			},
			quiverEntries: []domain.CollectionArrowEntry{
				{Path: "servers/cs2"},
				{Namespace: "github.com/owner/repo/arrow-a"},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	coll, err := m.ParseCollection([]byte("any"), domain.Namespace("github.com/char2cs/gaming.quiver"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(coll.Arrows) != 2 {
		t.Fatalf("expected 2 arrows, got %d", len(coll.Arrows))
	}
	if !coll.Arrows[0].IsLocal {
		t.Errorf("path: entry should be IsLocal=true, got false")
	}
	if coll.Arrows[1].IsLocal {
		t.Errorf("namespace: entry should be IsLocal=false, got true")
	}
}
