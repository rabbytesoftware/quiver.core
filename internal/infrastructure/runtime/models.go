package runtime

import "context"

// REEInterface defines the Runtime Execution Engine interface
// This interface provides comprehensive process execution and monitoring capabilities
type REEInterface interface {
	// Process Execution Methods
	Execute(
		ctx context.Context,
		path string,
		args []string,
	) (string, error)
	ExecuteWithTimeout(
		ctx context.Context,
		path string,
		args []string,
		timeoutSeconds int,
	) (string, error)
	ExecuteWithEnvironment(
		ctx context.Context,
		command []string,
		env map[string]string,
	) (string, error)

	// Process Management Methods
	StartProcess(
		ctx context.Context,
		path string,
		command []string,
		args []string,
	) (string, error)
	StopProcess(
		ctx context.Context,
		processID string,
	) error
	KillProcess(
		ctx context.Context,
		processID string,
	) error
	GetProcessStatus(
		ctx context.Context,
		processID string,
	) (string, error)
	ListProcesses(
		ctx context.Context,
	) ([]string, error)

	// Output Capture Methods
	CaptureOutput(
		ctx context.Context,
		processID string,
	) (string, error)
	CaptureError(
		ctx context.Context,
		processID string,
	) (string, error)
	StreamOutput(
		ctx context.Context,
		processID string,
	) (<-chan string, error)
	StreamError(
		ctx context.Context,
		processID string,
	) (<-chan string, error)

	// Pool Management Methods
	GetPoolSize(
		ctx context.Context,
	) (int, error)
	SetPoolSize(
		ctx context.Context,
		size int,
	) error
	GetAvailableExecutors(
		ctx context.Context,
	) (int, error)
	GetActiveExecutors(
		ctx context.Context,
	) (int, error)

	CleanupProcess(
		ctx context.Context,
		processID string,
	) error
	CleanupAllProcesses(
		ctx context.Context,
	) error
	Shutdown(
		ctx context.Context,
	) error
}
