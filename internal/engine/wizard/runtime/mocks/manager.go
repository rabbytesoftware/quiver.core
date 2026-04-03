package mocks

import "github.com/rabbytesoftware/quiver/internal/engine/wizard/runtime/process"

// Manager is a mock for builder.ProcessManager.
type Manager struct {
	Registered   []process.Process
	Unregistered []string
}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Register(p process.Process) {
	m.Registered = append(m.Registered, p)
}

func (m *Manager) Unregister(id string) {
	m.Unregistered = append(m.Unregistered, id)
}
