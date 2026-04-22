package mocks

import (
	"context"

	"github.com/rabbytesoftware/quiver/internal/domain"
)

type Execution struct {
	BeginExecutionErr error
	StopErr           error
	InstallErr        error
	UninstallErr      error
}

func (m *Execution) BeginExecution(ctx context.Context, ns, triggeredBy domain.Namespace, method string, userVars map[string]string) error {
	return m.BeginExecutionErr
}

func (m *Execution) Stop(ctx context.Context, ns domain.Namespace) error {
	return m.StopErr
}

func (m *Execution) Install(ctx context.Context, ns domain.Namespace, userVars map[string]string) error {
	return m.InstallErr
}

func (m *Execution) Uninstall(ctx context.Context, ns domain.Namespace, userVars map[string]string) error {
	return m.UninstallErr
}
