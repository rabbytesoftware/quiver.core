package manifold

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/assembler"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/models"
	"github.com/rabbytesoftware/quiver/internal/engine/manifold/resolver"
)

// stubResolver implements manifestResolver with configurable responses.
type stubResolver struct {
	arrowData []byte
	arrowErr  error
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

// ─── Constructor ─────────────────────────────────────────────────────────────

func TestNew_ReturnsManifoldInterface(t *testing.T) {
	var _ Manifold = New(0)
}

func TestNew_CustomTimeout(t *testing.T) {
	var _ Manifold = New(10 * time.Second)
}

// ─── ResolveArrow ─────────────────────────────────────────────────────────────

func TestResolveArrow_InvalidNamespace(t *testing.T) {
	m := New(5 * time.Second)
	_, err := m.ResolveArrow(context.Background(), domain.Namespace("not-valid"), "linux/amd64")
	if err == nil {
		t.Fatal("expected error for invalid namespace")
	}
}

func TestResolveArrow_UnsupportedPlatform(t *testing.T) {
	m := New(5 * time.Second)
	_, err := m.ResolveArrow(
		context.Background(),
		domain.Namespace("npm.js/user/repo"),
		"linux/amd64",
	)
	if err == nil {
		t.Fatal("expected error for unsupported platform")
	}
	if !errors.Is(err, resolver.ErrUnsupportedPlatform) {
		t.Errorf("expected ErrUnsupportedPlatform, got %v", err)
	}
}

func TestResolveArrow_ResolverError(t *testing.T) {
	resolveErr := errors.New("resolver failed")
	m := &manifold{
		resolver:    &stubResolver{arrowErr: resolveErr},
		assembler:   assembler.New(),
		parseArrow:  nil,
		parseQuiver: nil,
	}
	_, err := m.ResolveArrow(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
		"linux/amd64",
	)
	if !errors.Is(err, resolveErr) {
		t.Errorf("expected resolveErr, got %v", err)
	}
}

func TestResolveArrow_ParseError(t *testing.T) {
	parseErr := errors.New("parse failed")
	m := &manifold{
		resolver:  &stubResolver{arrowData: []byte("data")},
		assembler: assembler.New(),
		parseArrow: func(
			_ []byte,
		) (*models.RawArrow, error) {
			return nil, parseErr
		},
		parseQuiver: nil,
	}
	_, err := m.ResolveArrow(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
		"linux/amd64",
	)
	if !errors.Is(err, parseErr) {
		t.Errorf("expected parseErr, got %v", err)
	}
}

func TestResolveArrow_AssembleError(t *testing.T) {
	m := &manifold{
		resolver:  &stubResolver{arrowData: []byte("data")},
		assembler: assembler.New(),
		parseArrow: func(
			_ []byte,
		) (*models.RawArrow, error) {
			return &models.RawArrow{
				Metadata: models.RawMetadata{Name: "a", Version: "1.0.0"},
				Requirements: models.RawRequirements{
					OS: []string{"linux/amd64"},
				},
			}, nil
		},
		parseQuiver: nil,
	}
	_, err := m.ResolveArrow(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
		"windows/amd64", // unsupported OS triggers assembler error
	)
	if err == nil {
		t.Fatal("expected error for OS mismatch")
	}
}

func TestResolveArrow_Success(t *testing.T) {
	raw := &models.RawArrow{
		Metadata: models.RawMetadata{Name: "my-arrow", Version: "1.0.0"},
		Requirements: models.RawRequirements{
			CpuCores: 1,
			RamGB:    1,
			DiskGB:   1,
			OS:       []string{"linux/amd64"},
		},
		Lifecycle: models.RawLifecycle{
			Install:   []models.RawStep{{Type: "run", Command: "./install.sh"}},
			Uninstall: []models.RawStep{{Type: "run", Command: "./uninstall.sh"}},
		},
	}
	m := &manifold{
		resolver:  &stubResolver{arrowData: []byte("data")},
		assembler: assembler.New(),
		parseArrow: func(
			_ []byte,
		) (*models.RawArrow, error) {
			return raw, nil
		},
		parseQuiver: nil,
	}
	manifest, err := m.ResolveArrow(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
		"linux/amd64",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if manifest.Name != "my-arrow" {
		t.Errorf("Name = %q, want my-arrow", manifest.Name)
	}
}

// ─── ResolveQuiver ───────────────────────────────────────────────────────────

func TestResolveQuiver_InvalidNamespace(t *testing.T) {
	m := New(5 * time.Second)
	_, err := m.ResolveQuiver(context.Background(), domain.Namespace("not-valid"))
	if err == nil {
		t.Fatal("expected error for invalid namespace")
	}
}

func TestResolveQuiver_UnsupportedPlatform(t *testing.T) {
	m := New(5 * time.Second)
	_, err := m.ResolveQuiver(
		context.Background(),
		domain.Namespace("npm.js/user/repo"),
	)
	if err == nil {
		t.Fatal("expected error for unsupported platform")
	}
	if !errors.Is(err, resolver.ErrUnsupportedPlatform) {
		t.Errorf("expected ErrUnsupportedPlatform, got %v", err)
	}
}

func TestResolveQuiver_ResolverError(t *testing.T) {
	resolveErr := errors.New("resolver failed")
	m := &manifold{
		resolver:    &stubResolver{quiverErr: resolveErr},
		assembler:   assembler.New(),
		parseArrow:  nil,
		parseQuiver: nil,
	}
	_, err := m.ResolveQuiver(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if !errors.Is(err, resolveErr) {
		t.Errorf("expected resolveErr, got %v", err)
	}
}

func TestResolveQuiver_ParseError(t *testing.T) {
	parseErr := errors.New("parse failed")
	m := &manifold{
		resolver:   &stubResolver{quiverData: []byte("data")},
		assembler:  assembler.New(),
		parseArrow: nil,
		parseQuiver: func(
			_ []byte,
		) (*models.RawQuiver, error) {
			return nil, parseErr
		},
	}
	_, err := m.ResolveQuiver(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if !errors.Is(err, parseErr) {
		t.Errorf("expected parseErr, got %v", err)
	}
}

func TestResolveQuiver_Success(t *testing.T) {
	raw := &models.RawQuiver{
		Name:        "my-quiver",
		Description: "A test quiver",
	}
	m := &manifold{
		resolver:   &stubResolver{quiverData: []byte("data")},
		assembler:  assembler.New(),
		parseArrow: nil,
		parseQuiver: func(
			_ []byte,
		) (*models.RawQuiver, error) {
			return raw, nil
		},
	}
	manifest, err := m.ResolveQuiver(
		context.Background(),
		domain.Namespace("github.com/user/repo"),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if manifest.Name != "my-quiver" {
		t.Errorf("Name = %q, want my-quiver", manifest.Name)
	}
}
