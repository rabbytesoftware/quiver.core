package manifold

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
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
	arrowData       []byte
	arrowFilename   string
	arrowErr        error
	arrowAtData     []byte
	arrowAtFilename string
	arrowAtErr      error
	arrowAtPath     string // captures the path ResolveArrowAt was called with
	quiverData      []byte
	quiverErr       error
}

func (s *stubResolver) ResolveArrow(
	_ context.Context,
	_ domain.Namespace,
) ([]byte, string, error) {
	return s.arrowData, s.arrowFilename, s.arrowErr
}

func (s *stubResolver) ResolveArrowAt(
	_ context.Context,
	_ domain.Namespace,
	path string,
) ([]byte, string, error) {
	s.arrowAtPath = path
	return s.arrowAtData, s.arrowAtFilename, s.arrowAtErr
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
	readme        string
	readmeOK      bool
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

func (s *stubTranslator) ExtractReadme(data []byte) (string, bool) {
	return s.readme, s.readmeOK
}

func TestNew_ReturnsManifoldInterface(t *testing.T) {
	_ = New(0, hostedBy(&stubHost{}))
}

func TestNew_CustomTimeout(t *testing.T) {
	_ = New(10*time.Second, hostedBy(&stubHost{}))
}

// A manifold wired to no host lookup asks no host anything: every namespace
// resolves by cloning, and the latest-release step is a miss rather than a
// panic.
func TestNew_NilHostLookup_MissesEveryHostQuestion(t *testing.T) {
	crs := &stubConstraintResolver{result: "v1.10.0"}

	m := NewWithResolvers(&stubResolver{}, crs, nil)
	got, err := m.ResolveLatestStable(context.Background(), domain.Namespace("github.com/u/r"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.10.0" {
		t.Errorf("ref = %q, want %q", got, "v1.10.0")
	}
}

// A namespace on a host the lookup does not know skips the release step
// entirely: git tags answer for every host.
func TestResolveLatestStable_UnknownHostFallsBackToTags(t *testing.T) {
	crs := &stubConstraintResolver{result: "v1.10.0"}
	noHost := func(_ domain.Namespace) (Host, bool) { return nil, false }

	m := NewWithResolvers(&stubResolver{}, crs, noHost)
	got, err := m.ResolveLatestStable(context.Background(), domain.Namespace("git.example.test/u/r"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.10.0" {
		t.Errorf("ref = %q, want %q", got, "v1.10.0")
	}
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
	if !errors.Is(err, ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
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
			Name: "my-arrow",
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

func TestResolveArrowAt_Success(t *testing.T) {
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
			Name: "my-arrow",
		},
	}
	stub := &stubResolver{
		arrowAtData:     []byte("test"),
		arrowAtFilename: "tools/my-arrow.yaml",
	}
	m := &manifold{
		rsv: stub,
		trs: &stubTranslator{arrow: expectedManifest, precompiled: precompiled},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	result, raw, filename, err := m.ResolveArrowAt(
		context.Background(),
		domain.Namespace("github.com/rabbytesoftware/quiver.essentials/my-arrow"),
		"some/path",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "my-arrow" {
		t.Errorf("Name = %q, want my-arrow", result.Name)
	}
	if string(raw) != "test" {
		t.Errorf("raw = %q, want test", raw)
	}
	if filename != "tools/my-arrow.yaml" {
		t.Errorf("filename = %q, want tools/my-arrow.yaml", filename)
	}
	if stub.arrowAtPath != "some/path" {
		t.Errorf("ResolveArrowAt called with path %q, want some/path", stub.arrowAtPath)
	}
}

func TestResolveArrowAt_ResolverError(t *testing.T) {
	resolveErr := errors.New("resolver failed")
	m := &manifold{
		rsv: &stubResolver{arrowAtErr: resolveErr},
		trs: &stubTranslator{},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, _, _, err := m.ResolveArrowAt(
		context.Background(),
		domain.Namespace("github.com/rabbytesoftware/quiver.essentials/my-arrow"),
		"some/path",
	)
	if !errors.Is(err, resolveErr) {
		t.Errorf("expected resolveErr, got %v", err)
	}
}

func TestResolveArrowAt_TranslatorError(t *testing.T) {
	translateErr := errors.New("translator failed")
	m := &manifold{
		rsv: &stubResolver{arrowAtData: []byte("test")},
		trs: &stubTranslator{arrowErr: translateErr},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, _, _, err := m.ResolveArrowAt(
		context.Background(),
		domain.Namespace("github.com/rabbytesoftware/quiver.essentials/my-arrow"),
		"some/path",
	)
	if !errors.Is(err, translateErr) {
		t.Errorf("expected translateErr, got %v", err)
	}
	if !errors.Is(err, ErrInvalidManifest) {
		t.Errorf("expected ErrInvalidManifest, got %v", err)
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
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("expected ErrInvalidManifest, got %v", err)
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
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("expected ErrInvalidManifest, got %v", err)
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
			Name: "my-arrow",
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

func TestParseArrow_ExtractsReadme(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "10s", true)},
			},
		},
	}
	validManifest := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{Name: "my-arrow"},
	}
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{arrow: validManifest, precompiled: precompiled, readme: "# Docs", readmeOK: true},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	result, err := m.ParseArrow([]byte("any"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Readme != "# Docs" {
		t.Errorf("Readme = %q, want %q", result.Readme, "# Docs")
	}
}

func TestParseArrow_NoReadme_LeavesFieldEmpty(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "10s", true)},
			},
		},
	}
	validManifest := &domain.Arrow{
		ArrowMeta: domain.ArrowMeta{Name: "my-arrow"},
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
	if result.Readme != "" {
		t.Errorf("Readme = %q, want empty", result.Readme)
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
			Name: "my-arrow",
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
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("expected ErrInvalidManifest, got %v", err)
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
			Name: "my-arrow",
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
	if !errors.Is(err, compileErr) {
		t.Fatalf("expected compile error, got %v", err)
	}
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("expected ErrInvalidManifest, got %v", err)
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
	result       string
	err          error
	branchHash   string
	patterns     []string
	branch       string
	branchErr    error
	branchCall   int
	listTags     []string
	listTagsErr  error
	listTagsCall int
}

func (s *stubConstraintResolver) Resolve(_ context.Context, _ domain.Namespace, pattern string) (string, error) {
	s.patterns = append(s.patterns, pattern)
	return s.result, s.err
}

func (s *stubConstraintResolver) DefaultBranch(_ context.Context, _ domain.Namespace) (string, string, error) {
	s.branchCall++
	return s.branch, s.branchHash, s.branchErr
}

func (s *stubConstraintResolver) ListTags(_ context.Context, _ domain.Namespace) ([]string, error) {
	s.listTagsCall++
	return s.listTags, s.listTagsErr
}

// stubHost is a git host as manifold sees one. Only LatestRelease is ever asked
// here: the resolver is injected whole, so nothing in these tests reaches a raw
// file URL.
type stubHost struct {
	ref    string
	err    error
	called int
}

func (s *stubHost) LatestRelease(_ context.Context, _ domain.Namespace) (string, error) {
	s.called++
	return s.ref, s.err
}

func (s *stubHost) RawFileURL(
	_ domain.Namespace,
	_ string,
	_ string,
) (string, error) {
	return "", errors.New("the injected resolver fetches, not the host")
}

func (s *stubHost) DefaultBranches() []string { return nil }

// hostedBy answers for every namespace, which is what a manifold wired to a
// host it knows looks like.
func hostedBy(h *stubHost) HostLookup {
	return func(_ domain.Namespace) (Host, bool) { return h, true }
}

func TestNewWithResolvers_ReturnsManifoldInterface(t *testing.T) {
	rsv := &stubResolver{}
	crs := &stubConstraintResolver{}
	_ = NewWithResolvers(rsv, crs, hostedBy(&stubHost{}))
}

func TestNewWithResolvers_UsesInjectedResolver(t *testing.T) {
	resolveErr := errors.New("injected resolver failed")
	rsv := &stubResolver{arrowErr: resolveErr}
	crs := &stubConstraintResolver{}

	m := NewWithResolvers(rsv, crs, hostedBy(&stubHost{}))
	_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/user/repo"))

	if !errors.Is(err, resolveErr) {
		t.Fatalf("expected injected resolver error, got %v", err)
	}
}

// ─── ResolveLatestStable ──────────────────────────────────────────────────────

func TestResolveLatestStable_ReleasePermalinkWins(t *testing.T) {
	crs := &stubConstraintResolver{result: "v0.1.0"}

	m := NewWithResolvers(&stubResolver{}, crs, hostedBy(&stubHost{ref: "v2.96.0"}))
	got, err := m.ResolveLatestStable(context.Background(), domain.Namespace("github.com/cli/cli"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v2.96.0" {
		t.Errorf("ref = %q, want %q", got, "v2.96.0")
	}
	if len(crs.patterns) != 0 {
		t.Errorf("constraint resolver called %v, want no call", crs.patterns)
	}
}

func TestResolveLatestStable_FallsBackToTags(t *testing.T) {
	testCases := []struct {
		name string
		host *stubHost
	}{
		{
			name: "the host publishes no release",
			host: &stubHost{err: errors.New("no latest release")},
		},
		{
			name: "the host answers with an empty ref",
			host: &stubHost{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			crs := &stubConstraintResolver{result: "v1.10.0"}

			m := NewWithResolvers(&stubResolver{}, crs, hostedBy(tc.host))
			got, err := m.ResolveLatestStable(context.Background(), domain.Namespace("github.com/u/r"))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != "v1.10.0" {
				t.Errorf("ref = %q, want %q", got, "v1.10.0")
			}
			if tc.host.called != 1 {
				t.Errorf("host asked %d times, want 1", tc.host.called)
			}
			if len(crs.patterns) != 1 || crs.patterns[0] != "*" {
				t.Errorf("constraint patterns = %v, want [*]", crs.patterns)
			}
		})
	}
}

func TestResolveLatestStable_NoTagsIsAMiss(t *testing.T) {
	crs := &stubConstraintResolver{err: errors.New("no git tags match pattern")}

	m := NewWithResolvers(&stubResolver{}, crs, hostedBy(&stubHost{err: errors.New("no latest release")}))
	got, err := m.ResolveLatestStable(context.Background(), domain.Namespace("github.com/u/r"))

	if !errors.Is(err, ErrNoLatestStable) {
		t.Fatalf("expected ErrNoLatestStable, got %v", err)
	}
	if got != "" {
		t.Errorf("ref = %q, want empty", got)
	}
}

func TestResolveLatestStable_PrereleaseOnlyIsAMiss(t *testing.T) {
	testCases := []string{"nightly", "v2.0.0-rc.1", "latest"}

	for _, tag := range testCases {
		t.Run(tag, func(t *testing.T) {
			crs := &stubConstraintResolver{result: tag}

			m := NewWithResolvers(&stubResolver{}, crs, hostedBy(&stubHost{err: errors.New("no latest release")}))
			got, err := m.ResolveLatestStable(context.Background(), domain.Namespace("github.com/u/r"))

			if !errors.Is(err, ErrNoLatestStable) {
				t.Fatalf("expected ErrNoLatestStable, got %v", err)
			}
			if got != "" {
				t.Errorf("ref = %q, want empty", got)
			}
		})
	}
}

func TestResolveLatestInChannel_Stable_UsesLatestStablePath(t *testing.T) {
	crs := &stubConstraintResolver{result: "v1.10.0"}
	m := NewWithResolvers(&stubResolver{}, crs, hostedBy(&stubHost{}))

	got, err := m.ResolveLatestInChannel(context.Background(), domain.Namespace("github.com/u/r"), StableChannel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.10.0" {
		t.Errorf("got %q, want %q", got, "v1.10.0")
	}
}

func TestResolveLatestInChannel_Stable_UsesReleasePermalink(t *testing.T) {
	crs := &stubConstraintResolver{result: "v1.10.0"}
	m := NewWithResolvers(&stubResolver{}, crs, hostedBy(&stubHost{ref: "v2.96.0"}))

	got, err := m.ResolveLatestInChannel(context.Background(), domain.Namespace("github.com/u/r"), StableChannel)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v2.96.0" {
		t.Errorf("got %q, want %q", got, "v2.96.0")
	}
}

func TestResolveLatestInChannel_NonStable_PicksHighestInChannel(t *testing.T) {
	crs := &stubConstraintResolver{listTags: []string{"v1.2.0-rc1", "v1.2.0-rc2", "v1.4.0"}}
	m := NewWithResolvers(&stubResolver{}, crs, hostedBy(&stubHost{}))

	got, err := m.ResolveLatestInChannel(context.Background(), domain.Namespace("github.com/u/r"), "rc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "v1.2.0-rc2" {
		t.Errorf("got %q, want %q", got, "v1.2.0-rc2")
	}
}

func TestResolveLatestInChannel_NonStable_NeverAsksHostPermalink(t *testing.T) {
	host := &stubHost{ref: "v9.9.9"}
	crs := &stubConstraintResolver{listTags: []string{"v1.2.0-rc1"}}
	m := NewWithResolvers(&stubResolver{}, crs, hostedBy(host))

	if _, err := m.ResolveLatestInChannel(context.Background(), domain.Namespace("github.com/u/r"), "rc"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if host.called != 0 {
		t.Errorf("host.LatestRelease called %d times, want 0 — only \"stable\" has a permalink shortcut", host.called)
	}
}

func TestResolveLatestInChannel_NoTagInChannel_ReturnsErrNoTagInChannel(t *testing.T) {
	crs := &stubConstraintResolver{listTags: []string{"v1.4.0"}}
	m := NewWithResolvers(&stubResolver{}, crs, hostedBy(&stubHost{}))

	_, err := m.ResolveLatestInChannel(context.Background(), domain.Namespace("github.com/u/r"), "beta")
	if !errors.Is(err, ErrNoTagInChannel) {
		t.Fatalf("err = %v, want ErrNoTagInChannel", err)
	}
}

func TestResolveLatestInChannel_ListTagsError_Propagates(t *testing.T) {
	listErr := errors.New("dial tcp: connection refused")
	crs := &stubConstraintResolver{listTagsErr: listErr}
	m := NewWithResolvers(&stubResolver{}, crs, hostedBy(&stubHost{}))

	_, err := m.ResolveLatestInChannel(context.Background(), domain.Namespace("github.com/u/r"), "beta")
	if !errors.Is(err, listErr) {
		t.Fatalf("err = %v, want wrapping %v", err, listErr)
	}
}

// ─── ResolveDefaultBranch ─────────────────────────────────────────────────────

func TestResolveDefaultBranch_ReturnsWhateverHEADPointsAt(t *testing.T) {
	crs := &stubConstraintResolver{branch: "develop", branchHash: "abc123"}

	m := NewWithResolvers(&stubResolver{}, crs, hostedBy(&stubHost{}))
	got, hash, err := m.ResolveDefaultBranch(context.Background(), domain.Namespace("git.example.test/u/r"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "develop" {
		t.Errorf("branch = %q, want %q", got, "develop")
	}
	if hash != "abc123" {
		t.Errorf("hash = %q, want %q", hash, "abc123")
	}
	if crs.branchCall != 1 {
		t.Errorf("DefaultBranch called %d times, want 1", crs.branchCall)
	}
}

func TestResolveDefaultBranch_UnreachableRemoteIsAnError(t *testing.T) {
	crs := &stubConstraintResolver{branchErr: resolvers.ErrNoDefaultBranch}

	m := NewWithResolvers(&stubResolver{}, crs, hostedBy(&stubHost{}))
	got, hash, err := m.ResolveDefaultBranch(context.Background(), domain.Namespace("github.com/u/r"))

	if !errors.Is(err, resolvers.ErrNoDefaultBranch) {
		t.Fatalf("expected ErrNoDefaultBranch, got %v", err)
	}
	if got != "" {
		t.Errorf("branch = %q, want empty", got)
	}
	if hash != "" {
		t.Errorf("hash = %q, want empty", hash)
	}
}

func TestResolveConstraint_Success(t *testing.T) {
	rsv := &stubResolver{}
	crs := &stubConstraintResolver{result: "v1.2.3"}

	m := NewWithResolvers(rsv, crs, hostedBy(&stubHost{}))
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

	m := NewWithResolvers(rsv, crs, hostedBy(&stubHost{}))
	_, err := m.ResolveConstraint(context.Background(), domain.Namespace("github.com/user/repo@v1.*"), "v1.*")

	if !errors.Is(err, constraintErr) {
		t.Fatalf("expected constraintErr, got %v", err)
	}
}

func TestNewWithResolvers_ConstraintResolver_UsedOnResolveConstraint(t *testing.T) {
	_ = resolvers.NewConstraintResolver(0)

	rsv := &stubResolver{}
	crs := &stubConstraintResolver{result: "v2.0.0"}

	m := NewWithResolvers(rsv, crs, hostedBy(&stubHost{}))
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

// A local member lives inside the collection's repository, at the collection's
// own commit, so it is at the collection's ref and nothing else.
func TestParseCollection_VersionedNamespace_LocalArrowInheritsRef(t *testing.T) {
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
	want := domain.Namespace("github.com/char2cs/gaming.quiver/cs2@v1.2.3")
	if manifest.Arrows[0].Namespace != want {
		t.Errorf("Namespace = %q, want %q", manifest.Arrows[0].Namespace, want)
	}
}

// A ref written on the path is the collection's ref restated, so it is
// replaced by the derived one rather than believed.
func TestParseCollection_AuthoredPathRefIsReplacedByTheCollectionRef(t *testing.T) {
	testCases := []struct {
		name       string
		collection string
		want       domain.Namespace
	}{
		{
			name:       "collection carries a ref",
			collection: "github.com/char2cs/gaming.quiver@v2.0.0",
			want:       "github.com/char2cs/gaming.quiver/cs2@v2.0.0",
		},
		{
			name:       "collection is refless",
			collection: "github.com/char2cs/gaming.quiver",
			want:       "github.com/char2cs/gaming.quiver/cs2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := &manifold{
				rsv: &stubResolver{},
				trs: &stubTranslator{
					quiver: &domain.Collection{
						Meta: domain.CollectionMeta{Name: "Gaming", Description: "desc"},
					},
					quiverEntries: []domain.CollectionArrowEntry{
						{Path: "servers/cs2@v1.0.0"},
					},
				},
				cmp: compiler.New(),
				rls: ruleset.New(),
			}
			manifest, err := m.ParseCollection([]byte("any"), domain.Namespace(tc.collection))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if manifest.Arrows[0].Namespace != tc.want {
				t.Errorf("Namespace = %q, want %q", manifest.Arrows[0].Namespace, tc.want)
			}
		})
	}
}

// ValidateManifest parses against a refless dummy namespace, so a local member
// yields a refless namespace there. That is the correct answer for a pure
// syntax check: with no collection to be at, there is no ref to inherit.
func TestParseCollection_ReflessNamespace_LocalArrowStaysRefless(t *testing.T) {
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
	manifest, err := m.ParseCollection([]byte("any"), domain.Namespace("validation.dummy/collection"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := domain.Namespace("validation.dummy/collection/cs2")
	if manifest.Arrows[0].Namespace != want {
		t.Errorf("Namespace = %q, want %q", manifest.Arrows[0].Namespace, want)
	}
}

// A path that trims away to nothing has no last segment to name an arrow with.
func TestParseCollection_PathWithNoSegment_ReturnsError(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Name: "Gaming", Description: "desc"},
			},
			quiverEntries: []domain.CollectionArrowEntry{
				{Path: "/"},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, err := m.ParseCollection([]byte("any"), domain.Namespace("github.com/char2cs/gaming.quiver"))
	if err == nil {
		t.Fatal("expected error for a path with no namespace segment")
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

// ─── empty-command coverage across every lifecycle key ───────────────────────

// emptyCommandArrowYAML builds a schema-valid manifest whose only step in
// phase is a run step with no command at all. The step schema requires just
// `type`, so nothing upstream of the rules rejects it. install is emitted
// with a real command unless it is itself the phase under test, because
// LifecyclePairsRule requires install and uninstall to appear together.
func emptyCommandArrowYAML(phase string) []byte {
	install := `      install:
        - type: run
          command: echo installed
          title: Install
          timeout: 10s
          exit_on_failure: true
`
	if phase == "install" {
		install = ""
	}

	return []byte(`schema: "arrow@v0"
metadata:
  name: empty-command-probe
  description: test
targets:
  "*":
    lifecycle:
` + install + `      uninstall:
        - type: run
          command: echo uninstalled
          title: Uninstall
          timeout: 10s
          exit_on_failure: false
      ` + phase + `:
        - type: run
          title: detect
`)
}

// TestParseArrow_EmptyCommand_RejectedInPreinstalled is the fail-open this
// closes. OverrideableCoverageRule catches a command with no default and no OS
// coverage for every other lifecycle, but both it and its sibling
// OverrideableKeysRule enumerated only the five original keys, so a
// preinstalled step slipped past uncovered. It compiles to an empty command,
// which the run handler executes as `sh -c ""`, which exits 0 — a probe
// reporting a successful detection from a command that never ran, marking an
// arrow installed with nothing whatsoever verified behind it.
func TestParseArrow_EmptyCommand_RejectedInPreinstalled(t *testing.T) {
	m := New(time.Second, nil)

	_, err := m.ParseArrow(emptyCommandArrowYAML("preinstalled"))
	if err == nil {
		t.Fatal("expected a preinstalled run step with no command to be rejected, got nil")
	}
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("expected ErrInvalidManifest, got: %v", err)
	}
	if !strings.Contains(err.Error(), "lifecycle.preinstalled[0].command") {
		t.Fatalf("expected the error to name the uncovered preinstalled command, got: %v", err)
	}
	if !strings.Contains(err.Error(), "insufficient_coverage") {
		t.Fatalf("expected the coverage rule to be the one that rejected it, got: %v", err)
	}
}

// TestParseArrow_EmptyCommand_RejectedInInstall is the control the finding
// turns on: the identical step in an ordinary lifecycle was always rejected,
// and must stay rejected — the fix adds a key to a list, it does not change
// what the rule does to the keys already on it.
func TestParseArrow_EmptyCommand_RejectedInInstall(t *testing.T) {
	m := New(time.Second, nil)

	_, err := m.ParseArrow(emptyCommandArrowYAML("install"))
	if err == nil {
		t.Fatal("expected an install run step with no command to be rejected, got nil")
	}
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("expected ErrInvalidManifest, got: %v", err)
	}
	if !strings.Contains(err.Error(), "insufficient_coverage") {
		t.Fatalf("expected the coverage rule to be the one that rejected it, got: %v", err)
	}
}

// globKeyArrowYAML is the manifest §6.5 used to carry as its "BROKEN today"
// example: every step field covered by globs alone, with no default under any
// of them, plus an exports map covered the same way. It parses clean either
// way — OverrideableCoverageRule is glob-aware, so a glob-only field has always
// satisfied coverage — which is exactly what made the resolution gap silent.
func globKeyArrowYAML() []byte {
	return []byte(`schema: "arrow@v0"
metadata:
  name: glob-key-resolution
  description: test
targets:
  "*":
    exports:
      BIN:
        "linux/*": /usr/bin/foo
        "darwin/*": /usr/local/bin/foo
        "windows/*": C:\foo.exe
    lifecycle:
      install:
        - type: run
          title: Install
          timeout: 10s
          command:
            "linux/*": echo linux
            "darwin/*": echo darwin
            "windows/*": .\install.exe
      uninstall:
        - type: run
          command: echo bye
          title: Uninstall
          timeout: 10s
`)
}

// TestParseArrow_StepFieldGlobKey_ResolvesPerOS is the regression for the
// engine gap docs/spec/manifests/v0/arrow.md §6.5 used to document as
// permanent. Exports always resolved their Overrideable keys through
// selector.go's glob-aware resolveOverrideable; step fields went through
// resolveStepList → Step.Resolve → Overrideable.Resolve, a plain exact map
// lookup, so a `windows/*` key never matched `windows/amd64` and the field
// fell through to the Default — the empty string when there was none.
//
// The damage was silent: a glob-only command satisfies the coverage rule, so
// the manifest parsed clean and then handed `sh -c ""` to the shell, which
// exits 0 and reports success. This branch worked around it twice with exact
// keys and put a floor under `preinstalled` probes specifically; install,
// update, execute and stop had no floor at all.
//
// Every concrete GOOS/GOARCH is checked, not just the host's: the bug was
// per-target-OS by construction, and windows/amd64 and windows/arm64 are the
// exact pair that was proven broken.
func TestParseArrow_StepFieldGlobKey_ResolvesPerOS(t *testing.T) {
	m := New(time.Second, nil)
	arrow, err := m.ParseArrow(globKeyArrowYAML())
	if err != nil {
		t.Fatalf("a glob-only field satisfies the coverage rule, so this must parse: %v", err)
	}

	want := map[domain.OS]string{
		domain.OSWindowsAMD64: `.\install.exe`,
		domain.OSWindowsARM64: `.\install.exe`,
		domain.OSLinuxAMD64:   "echo linux",
		domain.OSLinuxARM64:   "echo linux",
		domain.OSDarwinAMD64:  "echo darwin",
		domain.OSDarwinARM64:  "echo darwin",
	}

	for os, wantCmd := range want {
		target, ok := arrow.Targets[os]
		if !ok {
			t.Fatalf("%s: the `*` target matches every OS, so one must have compiled", os)
		}
		run, ok := target.Lifecycle.Install[0].(step.RunStep)
		if !ok {
			t.Fatalf("%s: expected a run step, got %T", os, target.Lifecycle.Install[0])
		}
		if got := run.Command.Resolve(string(os)); got != wantCmd {
			t.Fatalf("%s: install command resolved to %q, want %q", os, got, wantCmd)
		}
	}
}

// Exports resolved globs before this change and must resolve them exactly the
// same way after it — the step fields were brought up to the exports
// behaviour, not the other way round.
func TestParseArrow_ExportGlobKey_StillResolvesPerOS(t *testing.T) {
	m := New(time.Second, nil)
	arrow, err := m.ParseArrow(globKeyArrowYAML())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	want := map[domain.OS]string{
		domain.OSWindowsAMD64: `C:\foo.exe`,
		domain.OSWindowsARM64: `C:\foo.exe`,
		domain.OSLinuxAMD64:   "/usr/bin/foo",
		domain.OSDarwinARM64:  "/usr/local/bin/foo",
	}

	for os, wantBin := range want {
		if got := arrow.Targets[os].Exports["BIN"]; got != wantBin {
			t.Fatalf("%s: export BIN resolved to %q, want %q", os, got, wantBin)
		}
	}
}

// Exact keys are the case every manifest in the ecosystem already uses,
// including the two places in this branch that worked around the gap with
// them. An exact key must still beat a glob that also matches, and the
// per-OS answers must be the ones the exact keys name.
func TestParseArrow_StepFieldExactKey_BeatsGlob(t *testing.T) {
	y := []byte(`schema: "arrow@v0"
metadata:
  name: exact-beats-glob
  description: test
targets:
  "*":
    lifecycle:
      install:
        - type: run
          title: Install
          timeout: 10s
          command:
            default: echo default
            "*": echo star
            "windows/*": echo any-windows
            windows/arm64: echo exactly-arm64
      uninstall:
        - type: run
          command: echo bye
          title: Uninstall
          timeout: 10s
`)

	m := New(time.Second, nil)
	arrow, err := m.ParseArrow(y)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	want := map[domain.OS]string{
		domain.OSWindowsARM64: "echo exactly-arm64",
		domain.OSWindowsAMD64: "echo any-windows",
		domain.OSLinuxAMD64:   "echo star",
	}

	for os, wantCmd := range want {
		run, ok := arrow.Targets[os].Lifecycle.Install[0].(step.RunStep)
		if !ok {
			t.Fatalf("%s: expected a run step", os)
		}
		if got := run.Command.Resolve(string(os)); got != wantCmd {
			t.Fatalf("%s: install command resolved to %q, want %q", os, got, wantCmd)
		}
	}
}

// A field with no key matching this OS at all still falls through to the
// Default, exactly as it always did.
func TestParseArrow_StepFieldNoMatchingKey_FallsBackToDefault(t *testing.T) {
	y := []byte(`schema: "arrow@v0"
metadata:
  name: glob-default-fallback
  description: test
targets:
  "*":
    lifecycle:
      install:
        - type: run
          title: Install
          timeout: 10s
          command:
            default: echo default
            "windows/*": echo windows
      uninstall:
        - type: run
          command: echo bye
          title: Uninstall
          timeout: 10s
`)

	m := New(time.Second, nil)
	arrow, err := m.ParseArrow(y)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	run, ok := arrow.Targets[domain.OSLinuxAMD64].Lifecycle.Install[0].(step.RunStep)
	if !ok {
		t.Fatalf("expected a run step")
	}
	if got := run.Command.Resolve(string(domain.OSLinuxAMD64)); got != "echo default" {
		t.Fatalf("install command resolved to %q, want the default", got)
	}
}

// Two globs of equal specificity matching the same GOOS/GOARCH is the
// ambiguity exports have always raised. A step field must raise the same one
// rather than picking a winner out of Go's map iteration order.
func TestParseArrow_StepFieldAmbiguousGlobKeys_Rejected(t *testing.T) {
	y := []byte(`schema: "arrow@v0"
metadata:
  name: ambiguous-step-glob
  description: test
targets:
  "*":
    lifecycle:
      install:
        - type: run
          title: Install
          timeout: 10s
          command:
            default: echo default
            "windows/*": echo by-os
            "*/amd64": echo by-arch
      uninstall:
        - type: run
          command: echo bye
          title: Uninstall
          timeout: 10s
`)

	m := New(time.Second, nil)
	_, err := m.ParseArrow(y)
	if err == nil {
		t.Fatal("expected windows/amd64 to be ambiguous between windows/* and */amd64")
	}
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("expected ErrInvalidManifest, got: %v", err)
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected the ambiguity to be named in the error, got: %v", err)
	}
}

// The same ambiguity on an export has always been rejected, and still is —
// both now travel the same resolver, so this pins that the step-field case
// did not get its own weaker treatment.
func TestParseArrow_ExportAmbiguousGlobKeys_StillRejected(t *testing.T) {
	y := []byte(`schema: "arrow@v0"
metadata:
  name: ambiguous-export-glob
  description: test
targets:
  "*":
    exports:
      BIN:
        default: /usr/bin/foo
        "windows/*": C:\by-os.exe
        "*/amd64": C:\by-arch.exe
    lifecycle:
      install:
        - type: run
          command: echo hi
          title: Install
          timeout: 10s
      uninstall:
        - type: run
          command: echo bye
          title: Uninstall
          timeout: 10s
`)

	m := New(time.Second, nil)
	_, err := m.ParseArrow(y)
	if err == nil {
		t.Fatal("expected windows/amd64 to be ambiguous between windows/* and */amd64")
	}
	if !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("expected ErrInvalidManifest, got: %v", err)
	}
}

// fetch and signal carry Overrideable fields too, and a glob key on any of
// them was just as broken. A single manifest covers the remaining field types
// on the one lifecycle that can hold all three.
func TestParseArrow_FetchAndSignalGlobKeys_Resolve(t *testing.T) {
	y := []byte(`schema: "arrow@v0"
metadata:
  name: glob-all-step-types
  description: test
targets:
  "*":
    lifecycle:
      install:
        - type: fetch
          title: Fetch
          timeout: 10s
          url:
            "windows/*": https://example.com/tool-windows.zip
            "linux/*": https://example.com/tool-linux.tar.gz
            "darwin/*": https://example.com/tool-darwin.tar.gz
          to:
            "*": ${WORKDIR}/tool
      execute:
        - type: run
          command: ./tool
          title: Run
          timeout: 10s
      stop:
        - type: signal
          title: Stop
          signal:
            "windows/*": kill
            "linux/*": graceful
            "darwin/*": graceful
          timeout:
            "*": 5s
      uninstall:
        - type: run
          command: echo bye
          title: Uninstall
          timeout: 10s
`)

	m := New(time.Second, nil)
	arrow, err := m.ParseArrow(y)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	win := arrow.Targets[domain.OSWindowsAMD64]
	fetch, ok := win.Lifecycle.Install[0].(step.FetchStep)
	if !ok {
		t.Fatalf("expected a fetch step, got %T", win.Lifecycle.Install[0])
	}
	if got := fetch.URL.Resolve(string(domain.OSWindowsAMD64)); got != "https://example.com/tool-windows.zip" {
		t.Fatalf("fetch url resolved to %q", got)
	}
	if got := fetch.To.Resolve(string(domain.OSWindowsAMD64)); got != "${WORKDIR}/tool" {
		t.Fatalf("fetch to resolved to %q", got)
	}

	sig, ok := win.Lifecycle.Stop[0].(step.SignalStep)
	if !ok {
		t.Fatalf("expected a signal step, got %T", win.Lifecycle.Stop[0])
	}
	if got := sig.Signal.Resolve(string(domain.OSWindowsAMD64)); got != step.SignalKindKill {
		t.Fatalf("windows signal resolved to %q, want kill", got)
	}
	if got := sig.Timeout.Resolve(string(domain.OSWindowsAMD64)); got != "5s" {
		t.Fatalf("signal timeout resolved to %q", got)
	}

	lin := arrow.Targets[domain.OSLinuxAMD64]
	linSig, ok := lin.Lifecycle.Stop[0].(step.SignalStep)
	if !ok {
		t.Fatalf("expected a signal step, got %T", lin.Lifecycle.Stop[0])
	}
	if got := linSig.Signal.Resolve(string(domain.OSLinuxAMD64)); got != step.SignalKindGraceful {
		t.Fatalf("linux signal resolved to %q, want graceful", got)
	}
}

// A custom method's steps go through the same resolver as a lifecycle's, and
// were broken in the same way.
func TestParseArrow_MethodStepGlobKey_Resolves(t *testing.T) {
	y := []byte(`schema: "arrow@v0"
metadata:
  name: glob-method-step
  description: test
targets:
  "*":
    lifecycle:
      install:
        - type: run
          command: echo hi
          title: Install
          timeout: 10s
      uninstall:
        - type: run
          command: echo bye
          title: Uninstall
          timeout: 10s
    methods:
      backup:
        available_in: [ready]
        steps:
          - type: run
            title: Backup
            timeout: 10s
            command:
              "windows/*": .\backup.exe
              "linux/*": ./backup
              "darwin/*": ./backup
`)

	m := New(time.Second, nil)
	arrow, err := m.ParseArrow(y)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	run, ok := arrow.Targets[domain.OSWindowsARM64].Methods["backup"].Steps[0].(step.RunStep)
	if !ok {
		t.Fatalf("expected a run step")
	}
	if got := run.Command.Resolve(string(domain.OSWindowsARM64)); got != `.\backup.exe` {
		t.Fatalf("method step command resolved to %q", got)
	}
}

func TestParseCollection_ExplicitAUID_OverridesLastSegment(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Name: "Essentials", Description: "desc"},
			},
			quiverEntries: []domain.CollectionArrowEntry{
				{Path: "tools/legacy/runtime-binary", AUID: "appimage-runtime"},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	ns := domain.Namespace("github.com/rabbytesoftware/quiver.essentials")
	manifest, err := m.ParseCollection([]byte("any"), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := domain.Namespace("github.com/rabbytesoftware/quiver.essentials/appimage-runtime")
	if manifest.Arrows[0].Namespace != want {
		t.Errorf("Namespace = %q, want %q", manifest.Arrows[0].Namespace, want)
	}
	if manifest.Arrows[0].SourcePath != "tools/legacy/runtime-binary" {
		t.Errorf("SourcePath = %q, want tools/legacy/runtime-binary", manifest.Arrows[0].SourcePath)
	}
}

func TestParseCollection_SourcePath_SetForImplicitAUID(t *testing.T) {
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
	manifest, err := m.ParseCollection([]byte("any"), domain.Namespace("github.com/char2cs/gaming.quiver"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if manifest.Arrows[0].SourcePath != "servers/cs2" {
		t.Errorf("SourcePath = %q, want servers/cs2", manifest.Arrows[0].SourcePath)
	}
}

func TestParseCollection_SourcePath_EmptyForExternalArrow(t *testing.T) {
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
	if manifest.Arrows[0].SourcePath != "" {
		t.Errorf("SourcePath = %q, want empty for an external arrow", manifest.Arrows[0].SourcePath)
	}
}

func TestParseCollection_AuthoredPathRef_StrippedFromSourcePath(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Name: "Gaming", Description: "desc"},
			},
			quiverEntries: []domain.CollectionArrowEntry{
				{Path: "servers/cs2@v1.0.0"},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	manifest, err := m.ParseCollection([]byte("any"), domain.Namespace("github.com/char2cs/gaming.quiver@v2.0.0"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if manifest.Arrows[0].SourcePath != "servers/cs2" {
		t.Errorf("SourcePath = %q, want servers/cs2 (ref suffix stripped)", manifest.Arrows[0].SourcePath)
	}
}

func TestResolveArrow_QuiverHosted_ResolvesViaOwningCollection(t *testing.T) {
	precompiled := map[string]models.PrecompiledTarget{
		"*": {
			Lifecycle: domain.TargetLifecycle{
				Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
				Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "10s", true)},
			},
		},
	}
	m := &manifold{
		rsv: &stubResolver{
			quiverData:      []byte("collection bytes"),
			arrowAtData:     []byte("arrow bytes"),
			arrowAtFilename: "tools/appimage-runtime.yaml",
		},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Name: "Essentials", Description: "desc"},
			},
			quiverEntries: []domain.CollectionArrowEntry{
				{Path: "tools/appimage-runtime", AUID: "appimage-runtime"},
			},
			arrow:       &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "appimage-runtime"}},
			precompiled: precompiled,
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	ns := domain.Namespace("github.com/rabbytesoftware/quiver.essentials/appimage-runtime")
	arrow, raw, filename, err := m.ResolveArrow(context.Background(), ns)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if arrow.Name != "appimage-runtime" {
		t.Errorf("Name = %q, want appimage-runtime", arrow.Name)
	}
	if string(raw) != "arrow bytes" {
		t.Errorf("raw = %q, want arrow bytes", raw)
	}
	if filename != "tools/appimage-runtime.yaml" {
		t.Errorf("filename = %q, want tools/appimage-runtime.yaml", filename)
	}
	stub := m.rsv.(*stubResolver)
	if stub.arrowAtPath != "tools/appimage-runtime" {
		t.Errorf("ResolveArrowAt called with path %q, want tools/appimage-runtime", stub.arrowAtPath)
	}
}

func TestResolveArrow_QuiverHosted_ArrowNotInCollection_ReturnsError(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{quiverData: []byte("collection bytes")},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Name: "Essentials", Description: "desc"},
			},
			quiverEntries: []domain.CollectionArrowEntry{
				{Path: "tools/other-tool"},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	ns := domain.Namespace("github.com/rabbytesoftware/quiver.essentials/appimage-runtime")
	_, _, _, err := m.ResolveArrow(context.Background(), ns)
	if !errors.Is(err, ErrArrowNotInCollection) {
		t.Fatalf("expected ErrArrowNotInCollection, got %v", err)
	}
}

func TestResolveArrow_QuiverHosted_ExternalEntrySameNamespace_DoesNotMatch(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{quiverData: []byte("collection bytes")},
		trs: &stubTranslator{
			quiver: &domain.Collection{
				Meta: domain.CollectionMeta{Name: "Essentials", Description: "desc"},
			},
			quiverEntries: []domain.CollectionArrowEntry{
				{Namespace: "github.com/rabbytesoftware/quiver.essentials/appimage-runtime"},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	ns := domain.Namespace("github.com/rabbytesoftware/quiver.essentials/appimage-runtime")
	_, _, _, err := m.ResolveArrow(context.Background(), ns)
	if !errors.Is(err, ErrArrowNotInCollection) {
		t.Fatalf("expected ErrArrowNotInCollection, got %v", err)
	}
	stub := m.rsv.(*stubResolver)
	if stub.arrowAtPath != "" {
		t.Errorf("ResolveArrowAt called with path %q, want it never called", stub.arrowAtPath)
	}
}

func TestResolveArrow_QuiverHosted_CollectionResolveFails_ReturnsError(t *testing.T) {
	collErr := errors.New("network down")
	m := &manifold{
		rsv: &stubResolver{quiverErr: collErr},
		trs: &stubTranslator{},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	ns := domain.Namespace("github.com/rabbytesoftware/quiver.essentials/appimage-runtime")
	_, _, _, err := m.ResolveArrow(context.Background(), ns)
	if !errors.Is(err, collErr) {
		t.Fatalf("expected wrapped collErr, got %v", err)
	}
}

func TestResolveArrow_NonQuiverHosted_SkipsCollectionLookup(t *testing.T) {
	m := &manifold{
		rsv: &stubResolver{arrowData: []byte("test"), arrowFilename: "ARROW.md"},
		trs: &stubTranslator{
			arrow: &domain.Arrow{ArrowMeta: domain.ArrowMeta{Name: "my-arrow"}},
			precompiled: map[string]models.PrecompiledTarget{
				"*": {
					Lifecycle: domain.TargetLifecycle{
						Install:   step.StepList{step.NewRunStep("install", "echo ok", false, "10s", true)},
						Uninstall: step.StepList{step.NewRunStep("uninstall", "echo bye", false, "10s", true)},
					},
				},
			},
		},
		cmp: compiler.New(),
		rls: ruleset.New(),
	}
	_, _, filename, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/user/repo"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filename != "ARROW.md" {
		t.Errorf("filename = %q, want ARROW.md", filename)
	}
}

func TestClassifyChannel_DelegatesToResolvers(t *testing.T) {
	testCases := []struct {
		name        string
		tag         string
		wantChannel string
		wantOK      bool
	}{
		{name: "stable tag", tag: "v1.4.0", wantChannel: "stable", wantOK: true},
		{name: "rc tag", tag: "v1.5.0-rc2", wantChannel: "rc", wantOK: true},
		{name: "pointer-shaped tag has no channel", tag: "nightly", wantChannel: "", wantOK: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			channel, ok := ClassifyChannel(tc.tag)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if channel != tc.wantChannel {
				t.Errorf("channel = %q, want %q", channel, tc.wantChannel)
			}
		})
	}
}

func TestListChannels_BucketsTagsAndIncludesDefaultBranch(t *testing.T) {
	crs := &stubConstraintResolver{
		listTags: []string{"v1.4.0", "v1.3.0", "v1.5.0-rc1", "v1.5.0-rc2", "nightly"},
		branch:   "main",
	}
	m := NewWithResolvers(&stubResolver{}, crs, hostedBy(&stubHost{}))

	got, err := m.ListChannels(context.Background(), domain.Namespace("github.com/u/r"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byName := make(map[string]ChannelInfo)
	for _, c := range got {
		byName[c.Name] = c
	}

	stable, ok := byName["stable"]
	if !ok {
		t.Fatal("missing stable channel")
	}
	if stable.Kind != "ordered" || stable.Latest != "v1.4.0" || stable.Count != 2 {
		t.Errorf("stable = %+v, want kind=ordered latest=v1.4.0 count=2", stable)
	}

	rc, ok := byName["rc"]
	if !ok {
		t.Fatal("missing rc channel")
	}
	if rc.Kind != "ordered" || rc.Latest != "v1.5.0-rc2" || rc.Count != 2 {
		t.Errorf("rc = %+v, want kind=ordered latest=v1.5.0-rc2 count=2", rc)
	}
	if got, want := rc.Members, []string{"v1.5.0-rc2", "v1.5.0-rc1"}; !slices.Equal(got, want) {
		t.Errorf("rc.Members = %v, want %v", got, want)
	}

	nightlyTag, ok := byName["nightly"]
	if !ok {
		t.Fatal("missing nightly pointer channel from the tag")
	}
	if nightlyTag.Kind != "pointer" || nightlyTag.Latest != "nightly" {
		t.Errorf("nightly tag channel = %+v, want kind=pointer latest=nightly", nightlyTag)
	}

	mainBranch, ok := byName["main"]
	if !ok {
		t.Fatal("missing default branch as a pointer channel")
	}
	if mainBranch.Kind != "pointer" {
		t.Errorf("main branch channel = %+v, want kind=pointer", mainBranch)
	}

	if len(got) == 0 || got[0].Name != StableChannel {
		t.Errorf("got[0].Name = %q, want stable first", got[0].Name)
	}
}

// TestListChannels_DeterministicOrderAcrossRepeatedCalls guards against Go's
// randomized map iteration leaking into ListChannels's result order.
// ListChannels used to build its result by ranging over an internal
// map[string][]string bucket, so the returned slice's order was empirically
// observed to differ across otherwise-identical calls -- a real bug, since
// this endpoint exists specifically to back a UI dropdown that needs stable
// ordering. Map iteration randomization is probabilistic, not guaranteed on
// any single run, so this calls ListChannels many times on the same input
// and requires every result to match the first one exactly, in order.
func TestListChannels_DeterministicOrderAcrossRepeatedCalls(t *testing.T) {
	crs := &stubConstraintResolver{
		listTags: []string{"v1.4.0", "v1.3.0", "v1.5.0-rc1", "v1.5.0-rc2", "nightly"},
		branch:   "main",
	}
	m := NewWithResolvers(&stubResolver{}, crs, hostedBy(&stubHost{}))

	const runs = 20
	var first []ChannelInfo
	for i := 0; i < runs; i++ {
		got, err := m.ListChannels(context.Background(), domain.Namespace("github.com/u/r"))
		if err != nil {
			t.Fatalf("run %d: unexpected error: %v", i, err)
		}
		if i == 0 {
			first = got
			if len(first) == 0 || first[0].Name != StableChannel {
				t.Fatalf("run %d: stable must be first, got %+v", i, first)
			}
			continue
		}
		if !reflect.DeepEqual(first, got) {
			t.Fatalf("run %d: order differs from run 0\nrun 0: %+v\nrun %d: %+v", i, first, i, got)
		}
	}
}

func TestListChannels_NoDefaultBranch_StillReturnsTagChannels(t *testing.T) {
	crs := &stubConstraintResolver{
		listTags:  []string{"v1.0.0"},
		branchErr: resolvers.ErrNoDefaultBranch,
	}
	m := NewWithResolvers(&stubResolver{}, crs, hostedBy(&stubHost{}))

	got, err := m.ListChannels(context.Background(), domain.Namespace("github.com/u/r"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "stable" {
		t.Errorf("got %+v, want exactly one stable channel", got)
	}
}

func TestListChannels_ListTagsError_Propagates(t *testing.T) {
	listErr := errors.New("dial tcp: connection refused")
	crs := &stubConstraintResolver{listTagsErr: listErr}
	m := NewWithResolvers(&stubResolver{}, crs, hostedBy(&stubHost{}))

	_, err := m.ListChannels(context.Background(), domain.Namespace("github.com/u/r"))
	if !errors.Is(err, listErr) {
		t.Fatalf("err = %v, want wrapping %v", err, listErr)
	}
}
