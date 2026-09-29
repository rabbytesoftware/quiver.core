//go:build windows

package userpath

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	environmentKey   = "Environment"
	pathValue        = "Path"
	hwndBroadcast    = 0xffff
	wmSettingChange  = 0x001a
	smtoAbortIfHung  = 0x0002
	broadcastTimeout = 5000
)

type registryUserPath struct {
	root   registry.Key
	subkey string
	area   string
}

func New() UserPath {
	return &registryUserPath{root: registry.CURRENT_USER, subkey: environmentKey, area: environmentKey}
}

func (r *registryUserPath) Read() (string, error) {
	k, err := registry.OpenKey(r.root, r.subkey, registry.QUERY_VALUE)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", r.subkey, err)
	}
	defer func() { _ = k.Close() }()

	value, _, err := k.GetStringValue(pathValue)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", r.Location(), err)
	}
	return value, nil
}

// Write keeps the value's registry type: a REG_SZ Path stays REG_SZ, anything
// new or REG_EXPAND_SZ is written as REG_EXPAND_SZ, so %VAR% entries still
// expand.
func (r *registryUserPath) Write(
	value string,
) error {
	k, err := registry.OpenKey(r.root, r.subkey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("open %s: %w", r.subkey, err)
	}
	defer func() { _ = k.Close() }()

	_, kind, err := k.GetStringValue(pathValue)
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("read %s: %w", r.Location(), err)
	}

	set := k.SetExpandStringValue
	if err == nil && kind == registry.SZ {
		set = k.SetStringValue
	}
	if err := set(pathValue, value); err != nil {
		return fmt.Errorf("write %s: %w", r.Location(), err)
	}
	return nil
}

func (r *registryUserPath) Broadcast() error {
	area, err := windows.UTF16PtrFromString(r.area)
	if err != nil {
		return fmt.Errorf("broadcast %s: %w", r.area, err)
	}

	proc := windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
	if err := proc.Find(); err != nil {
		return fmt.Errorf("broadcast %s: %w", r.area, err)
	}

	var result uintptr
	ret, _, callErr := proc.Call(
		hwndBroadcast,
		wmSettingChange,
		0,
		uintptr(unsafe.Pointer(area)), // #nosec G103 -- SendMessageTimeoutW takes the setting name as a UTF-16 pointer
		smtoAbortIfHung,
		broadcastTimeout,
		uintptr(unsafe.Pointer(&result)), // #nosec G103 -- SendMessageTimeoutW writes its result through this pointer
	)
	if ret == 0 {
		return fmt.Errorf("broadcast %s: %w", r.area, callErr)
	}
	return nil
}

func (r *registryUserPath) Location() string {
	return `HKCU\` + r.subkey + `\` + pathValue
}
