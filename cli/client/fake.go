package client

import "context"

// FakeClient implements QuiverClient for use in unit tests.
// Set only the Fn fields your test needs; unset fields return zero values.
// Lifecycle and watch methods with unset Fn return a closed, empty channel.
type FakeClient struct {
	ArrowListFn        func(ctx context.Context, userInstalled bool) ([]ArrowListItem, error)
	ArrowGetFn         func(ctx context.Context, ns string) (*ArrowDetail, error)
	ArrowGetManifestFn func(ctx context.Context, ns string) ([]byte, error)
	ArrowAddFn         func(ctx context.Context, ns string) error
	ArrowUpdateFn      func(ctx context.Context, ns string) error
	ArrowRemoveFn      func(ctx context.Context, ns string) error
	ArrowSeedFn        func(ctx context.Context, ns string, manifest []byte) error
	ArrowValidateFn    func(ctx context.Context, ns string, manifest []byte) (*ValidationResult, error)

	InstallFn   func(ctx context.Context, ns string, vars map[string]string) (<-chan ArrowRuntime, error)
	UninstallFn func(ctx context.Context, ns string, vars map[string]string) (<-chan ArrowRuntime, error)
	ExecuteFn   func(ctx context.Context, ns string, vars map[string]string) (<-chan ArrowRuntime, error)
	StopFn      func(ctx context.Context, ns string) (<-chan ArrowRuntime, error)
	UpdateFn    func(ctx context.Context, ns string) (<-chan ArrowRuntime, error)
	RunMethodFn func(ctx context.Context, ns, method string, vars map[string]string) (<-chan ArrowRuntime, error)

	RuntimeGetFn   func(ctx context.Context, ns string) (*ArrowRuntime, error)
	RuntimeListFn  func(ctx context.Context) ([]ArrowRuntime, error)
	WatchRuntimeFn func(ctx context.Context, ns string) (<-chan ArrowRuntime, error)

	CollectionListFn   func(ctx context.Context) ([]Collection, error)
	CollectionGetFn    func(ctx context.Context, ns string) (*Collection, error)
	CollectionAddFn    func(ctx context.Context, ns string) error
	CollectionUpdateFn func(ctx context.Context, ns string) error
	CollectionRemoveFn func(ctx context.Context, ns string) error

	HealthFn func(ctx context.Context) (*HealthStatus, error)
}

// StreamOf returns a closed, buffered channel pre-loaded with the given snapshots.
// Use it to script lifecycle and watch responses in unit tests.
func StreamOf(snapshots ...ArrowRuntime) <-chan ArrowRuntime {
	ch := make(chan ArrowRuntime, len(snapshots))
	for _, s := range snapshots {
		ch <- s
	}
	close(ch)
	return ch
}

func closedCh() <-chan ArrowRuntime {
	ch := make(chan ArrowRuntime)
	close(ch)
	return ch
}

func (f *FakeClient) ArrowList(ctx context.Context, userInstalled bool) ([]ArrowListItem, error) {
	if f.ArrowListFn != nil {
		return f.ArrowListFn(ctx, userInstalled)
	}
	return nil, nil
}

func (f *FakeClient) ArrowGet(ctx context.Context, ns string) (*ArrowDetail, error) {
	if f.ArrowGetFn != nil {
		return f.ArrowGetFn(ctx, ns)
	}
	return nil, nil
}

func (f *FakeClient) ArrowGetManifest(ctx context.Context, ns string) ([]byte, error) {
	if f.ArrowGetManifestFn != nil {
		return f.ArrowGetManifestFn(ctx, ns)
	}
	return nil, nil
}

func (f *FakeClient) ArrowAdd(ctx context.Context, ns string) error {
	if f.ArrowAddFn != nil {
		return f.ArrowAddFn(ctx, ns)
	}
	return nil
}

func (f *FakeClient) ArrowUpdate(ctx context.Context, ns string) error {
	if f.ArrowUpdateFn != nil {
		return f.ArrowUpdateFn(ctx, ns)
	}
	return nil
}

func (f *FakeClient) ArrowRemove(ctx context.Context, ns string) error {
	if f.ArrowRemoveFn != nil {
		return f.ArrowRemoveFn(ctx, ns)
	}
	return nil
}

func (f *FakeClient) ArrowSeed(ctx context.Context, ns string, manifest []byte) error {
	if f.ArrowSeedFn != nil {
		return f.ArrowSeedFn(ctx, ns, manifest)
	}
	return nil
}

func (f *FakeClient) ArrowValidate(ctx context.Context, ns string, manifest []byte) (*ValidationResult, error) {
	if f.ArrowValidateFn != nil {
		return f.ArrowValidateFn(ctx, ns, manifest)
	}
	return nil, nil
}

func (f *FakeClient) Install(ctx context.Context, ns string, vars map[string]string) (<-chan ArrowRuntime, error) {
	if f.InstallFn != nil {
		return f.InstallFn(ctx, ns, vars)
	}
	return closedCh(), nil
}

func (f *FakeClient) Uninstall(ctx context.Context, ns string, vars map[string]string) (<-chan ArrowRuntime, error) {
	if f.UninstallFn != nil {
		return f.UninstallFn(ctx, ns, vars)
	}
	return closedCh(), nil
}

func (f *FakeClient) Execute(ctx context.Context, ns string, vars map[string]string) (<-chan ArrowRuntime, error) {
	if f.ExecuteFn != nil {
		return f.ExecuteFn(ctx, ns, vars)
	}
	return closedCh(), nil
}

func (f *FakeClient) Stop(ctx context.Context, ns string) (<-chan ArrowRuntime, error) {
	if f.StopFn != nil {
		return f.StopFn(ctx, ns)
	}
	return closedCh(), nil
}

func (f *FakeClient) Update(ctx context.Context, ns string) (<-chan ArrowRuntime, error) {
	if f.UpdateFn != nil {
		return f.UpdateFn(ctx, ns)
	}
	return closedCh(), nil
}

func (f *FakeClient) RunMethod(ctx context.Context, ns, method string, vars map[string]string) (<-chan ArrowRuntime, error) {
	if f.RunMethodFn != nil {
		return f.RunMethodFn(ctx, ns, method, vars)
	}
	return closedCh(), nil
}

func (f *FakeClient) RuntimeGet(ctx context.Context, ns string) (*ArrowRuntime, error) {
	if f.RuntimeGetFn != nil {
		return f.RuntimeGetFn(ctx, ns)
	}
	return nil, nil
}

func (f *FakeClient) RuntimeList(ctx context.Context) ([]ArrowRuntime, error) {
	if f.RuntimeListFn != nil {
		return f.RuntimeListFn(ctx)
	}
	return nil, nil
}

func (f *FakeClient) WatchRuntime(ctx context.Context, ns string) (<-chan ArrowRuntime, error) {
	if f.WatchRuntimeFn != nil {
		return f.WatchRuntimeFn(ctx, ns)
	}
	return closedCh(), nil
}

func (f *FakeClient) CollectionList(ctx context.Context) ([]Collection, error) {
	if f.CollectionListFn != nil {
		return f.CollectionListFn(ctx)
	}
	return nil, nil
}

func (f *FakeClient) CollectionGet(ctx context.Context, ns string) (*Collection, error) {
	if f.CollectionGetFn != nil {
		return f.CollectionGetFn(ctx, ns)
	}
	return nil, nil
}

func (f *FakeClient) CollectionAdd(ctx context.Context, ns string) error {
	if f.CollectionAddFn != nil {
		return f.CollectionAddFn(ctx, ns)
	}
	return nil
}

func (f *FakeClient) CollectionUpdate(ctx context.Context, ns string) error {
	if f.CollectionUpdateFn != nil {
		return f.CollectionUpdateFn(ctx, ns)
	}
	return nil
}

func (f *FakeClient) CollectionRemove(ctx context.Context, ns string) error {
	if f.CollectionRemoveFn != nil {
		return f.CollectionRemoveFn(ctx, ns)
	}
	return nil
}

func (f *FakeClient) Health(ctx context.Context) (*HealthStatus, error) {
	if f.HealthFn != nil {
		return f.HealthFn(ctx)
	}
	return nil, nil
}
