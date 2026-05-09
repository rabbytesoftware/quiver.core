package dependencies_test

import (
	"context"
	"errors"
	"testing"

	domainstep "github.com/rabbytesoftware/quiver.core/internal/domain/runtime/step"
	wizstep "github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step"
	"github.com/rabbytesoftware/quiver.core/internal/engine/wizard/internal/step/dependencies"
)

func TestHandler_NilExecutor_NoOp(t *testing.T) {
	h := dependencies.NewHandler(nil)

	err := h.Execute(context.Background(), wizstep.Request{}, domainstep.NewDependenciesStep("pkg"))
	if err != nil {
		t.Errorf("Execute(nil executor) = %v, want nil", err)
	}
}

func TestHandler_CallsExecutor(t *testing.T) {
	called := false
	h := dependencies.NewHandler(func(_ context.Context, _ wizstep.Request, _ domainstep.DependenciesStep) error {
		called = true
		return nil
	})

	err := h.Execute(context.Background(), wizstep.Request{}, domainstep.NewDependenciesStep("pkg"))
	if err != nil {
		t.Errorf("Execute = %v, want nil", err)
	}
	if !called {
		t.Error("executor was not called")
	}
}

func TestHandler_PropagatesError(t *testing.T) {
	wantErr := errors.New("dep error")
	h := dependencies.NewHandler(func(_ context.Context, _ wizstep.Request, _ domainstep.DependenciesStep) error {
		return wantErr
	})

	err := h.Execute(context.Background(), wizstep.Request{}, domainstep.NewDependenciesStep("pkg"))
	if !errors.Is(err, wantErr) {
		t.Errorf("Execute = %v, want %v", err, wantErr)
	}
}

func TestHandler_PassesRequestAndStep(t *testing.T) {
	var gotNS string
	var gotTitle string

	h := dependencies.NewHandler(func(_ context.Context, req wizstep.Request, s domainstep.DependenciesStep) error {
		gotNS = req.NSKey
		gotTitle = s.Title()
		return nil
	})

	req := wizstep.Request{NSKey: "test/ns"}
	step := domainstep.NewDependenciesStep("my-pkg")

	_ = h.Execute(context.Background(), req, step)

	if gotNS != "test/ns" {
		t.Errorf("NSKey = %q, want %q", gotNS, "test/ns")
	}
	if gotTitle != "my-pkg" {
		t.Errorf("Title = %q, want %q", gotTitle, "my-pkg")
	}
}
