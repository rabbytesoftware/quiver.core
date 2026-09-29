package userpath

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakePath struct {
	value        string
	readErr      error
	writeErr     error
	broadcastErr error
	writes       int
	broadcasts   int
}

func (f *fakePath) Read() (string, error) {
	return f.value, f.readErr
}

func (f *fakePath) Write(
	value string,
) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.writes++
	f.value = value
	return nil
}

func (f *fakePath) Broadcast() error {
	f.broadcasts++
	return f.broadcastErr
}

func (*fakePath) Location() string {
	return "fake"
}

func TestEditor_Append(t *testing.T) {
	testCases := []struct {
		name       string
		current    string
		dir        string
		want       string
		wantWrites int
	}{
		{name: "empty", current: "", dir: `C:\q`, want: `C:\q`, wantWrites: 1},
		{name: "appends, never prepends", current: `C:\a;%USERPROFILE%\b`, dir: `C:\q`, want: `C:\a;%USERPROFILE%\b;C:\q`, wantWrites: 1},
		{name: "trailing separator", current: `C:\a;`, dir: `C:\q`, want: `C:\a;C:\q`, wantWrites: 1},
		{name: "already present in another case", current: `C:\a;c:\Q`, dir: `C:\q`, want: `C:\a;c:\Q`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakePath{value: tc.current}
			e := NewEditor(fake)

			require.NoError(t, e.Append(context.Background(), tc.dir))
			present, err := e.Contains(tc.dir)
			require.NoError(t, err)

			assert.True(t, present)
			assert.Equal(t, tc.want, fake.value)
			assert.Equal(t, tc.wantWrites, fake.writes)
			assert.Equal(t, tc.wantWrites, fake.broadcasts)
			assert.Equal(t, "fake", e.Location())
		})
	}
}

func TestEditor_Drop(t *testing.T) {
	testCases := []struct {
		name       string
		current    string
		want       string
		wantWrites int
	}{
		{name: "drops matching entries and keeps the rest verbatim", current: `C:\a;;C:\drop\x;%Q%\b;C:\drop\y`, want: `C:\a;;%Q%\b`, wantWrites: 1},
		{name: "nothing to drop writes nothing", current: `C:\a;C:\b`, want: `C:\a;C:\b`},
		{name: "empty", current: "", want: ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakePath{value: tc.current}

			err := NewEditor(fake).Drop(context.Background(), func(entry string) bool {
				return strings.HasPrefix(entry, `C:\drop`)
			})

			require.NoError(t, err)
			assert.Equal(t, tc.want, fake.value)
			assert.Equal(t, tc.wantWrites, fake.writes)
			assert.Equal(t, tc.wantWrites, fake.broadcasts)
		})
	}
}

func TestEditor_Errors(t *testing.T) {
	boom := errors.New("boom")
	dropAll := func(string) bool { return true }

	readFails := NewEditor(&fakePath{readErr: boom})
	_, err := readFails.Contains(`C:\q`)
	assert.ErrorIs(t, err, boom)
	assert.ErrorIs(t, readFails.Append(context.Background(), `C:\q`), boom)
	assert.ErrorIs(t, readFails.Drop(context.Background(), dropAll), boom)

	writeFails := NewEditor(&fakePath{value: `C:\a`, writeErr: boom})
	assert.ErrorIs(t, writeFails.Append(context.Background(), `C:\q`), boom)
	assert.ErrorIs(t, writeFails.Drop(context.Background(), dropAll), boom)
}

func TestEditor_BroadcastFailureStillSucceeds(t *testing.T) {
	fake := &fakePath{broadcastErr: errors.New("hung")}

	require.NoError(t, NewEditor(fake).Append(context.Background(), `C:\q`))

	assert.Equal(t, `C:\q`, fake.value)
	assert.Equal(t, 1, fake.broadcasts)
}

func TestEditor_ConcurrentAppendsKeepEveryEntry(t *testing.T) {
	fake := &fakePath{}
	e := NewEditor(fake)
	dirs := []string{`C:\a`, `C:\b`, `C:\c`, `C:\d`, `C:\e`, `C:\f`, `C:\g`, `C:\h`}

	var wg sync.WaitGroup
	for _, dir := range dirs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, e.Append(context.Background(), dir))
		}()
	}
	wg.Wait()

	for _, dir := range dirs {
		assert.True(t, Contains(fake.value, dir, Separator, true), dir)
	}
}
