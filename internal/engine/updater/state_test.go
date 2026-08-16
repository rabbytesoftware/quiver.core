package updater

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestState(t *testing.T) (Layout, Artifact, Artifact) {
	t.Helper()
	layout, err := newLayout(t.TempDir(), "linux")
	require.NoError(t, err)
	current, err := NewArtifact(layout, "v1.0.0", testDigest)
	require.NoError(t, err)
	previousDigest := "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	previous, err := NewArtifact(layout, "v0.9.0", previousDigest)
	require.NoError(t, err)
	return layout, current, previous
}

func TestSelection_Validate_ValidSelectionReturnsNil(t *testing.T) {
	layout, current, previous := newTestState(t)
	selection := Selection{
		Schema:     StateSchema,
		Generation: 2,
		Current:    current,
		Previous:   &previous,
		PromotedAt: time.Now().UTC(),
	}

	assert.NoError(t, selection.Validate(layout))
}

func TestSelection_Validate_FirstSelectionWithoutPreviousReturnsNil(t *testing.T) {
	layout, current, _ := newTestState(t)
	selection := Selection{
		Schema:     StateSchema,
		Generation: 1,
		Current:    current,
		PromotedAt: time.Now().UTC(),
	}

	assert.NoError(t, selection.Validate(layout))
}

func TestSelection_Validate_InvalidFieldsReturnError(t *testing.T) {
	layout, current, previous := newTestState(t)
	now := time.Now().UTC()
	testCases := []struct {
		name      string
		selection Selection
		want      error
	}{
		{name: "schema", selection: Selection{Generation: 1, Current: current, PromotedAt: now}, want: ErrInvalidState},
		{name: "generation", selection: Selection{Schema: StateSchema, Current: current, PromotedAt: now}, want: ErrInvalidState},
		{name: "current", selection: Selection{Schema: StateSchema, Generation: 1, PromotedAt: now}, want: ErrInvalidState},
		{name: "promoted at", selection: Selection{Schema: StateSchema, Generation: 1, Current: current}, want: ErrInvalidState},
		{name: "previous", selection: Selection{Schema: StateSchema, Generation: 2, Current: current, Previous: &Artifact{}, PromotedAt: now}, want: ErrInvalidState},
		{name: "duplicate previous", selection: Selection{Schema: StateSchema, Generation: 2, Current: current, Previous: &current, PromotedAt: now}, want: ErrInvalidState},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.selection.Validate(layout)

			require.Error(t, err)
			assert.True(t, errors.Is(err, tc.want))
		})
	}
	_ = previous
}

func TestStaged_Validate_ValidStateReturnsNil(t *testing.T) {
	layout, current, _ := newTestState(t)
	staged := Staged{Schema: StateSchema, Candidate: current, PreparedAt: time.Now().UTC()}

	assert.NoError(t, staged.Validate(layout))
}

func TestStaged_Validate_InvalidFieldsReturnError(t *testing.T) {
	layout, current, _ := newTestState(t)
	now := time.Now().UTC()
	testCases := []struct {
		name   string
		staged Staged
	}{
		{name: "schema", staged: Staged{Candidate: current, PreparedAt: now}},
		{name: "candidate", staged: Staged{Schema: StateSchema, PreparedAt: now}},
		{name: "prepared at", staged: Staged{Schema: StateSchema, Candidate: current}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.staged.Validate(layout)

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidState)
		})
	}
}

func TestSelection_EncodeDecode_RoundTrips(t *testing.T) {
	layout, current, previous := newTestState(t)
	selection := Selection{
		Schema:     StateSchema,
		Generation: 2,
		Current:    current,
		Previous:   &previous,
		PromotedAt: time.Now().UTC().Truncate(time.Second),
	}

	data, err := EncodeSelection(layout, selection)
	require.NoError(t, err)
	decoded, err := DecodeSelection(layout, data)

	require.NoError(t, err)
	assert.Equal(t, selection, decoded)
	assert.Equal(t, byte('\n'), data[len(data)-1])
}

func TestSelection_Encode_InvalidStateReturnsError(t *testing.T) {
	layout, _, _ := newTestState(t)

	data, err := EncodeSelection(layout, Selection{})

	require.ErrorIs(t, err, ErrInvalidState)
	assert.Nil(t, data)
}

func TestSelection_Decode_InvalidDocumentsReturnError(t *testing.T) {
	layout, _, _ := newTestState(t)
	testCases := []struct {
		name string
		data string
	}{
		{name: "malformed", data: "{"},
		{name: "unknown field", data: `{"schema":1,"unknown":true}`},
		{name: "multiple values", data: `{} {}`},
		{name: "invalid state", data: `{}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			selection, err := DecodeSelection(layout, []byte(tc.data))

			require.Error(t, err)
			assert.Empty(t, selection)
		})
	}
}

func TestStaged_EncodeDecode_RoundTrips(t *testing.T) {
	layout, current, _ := newTestState(t)
	staged := Staged{
		Schema:     StateSchema,
		Candidate:  current,
		PreparedAt: time.Now().UTC().Truncate(time.Second),
	}

	data, err := EncodeStaged(layout, staged)
	require.NoError(t, err)
	decoded, err := DecodeStaged(layout, data)

	require.NoError(t, err)
	assert.Equal(t, staged, decoded)
}

func TestStaged_Encode_InvalidStateReturnsError(t *testing.T) {
	layout, _, _ := newTestState(t)

	data, err := EncodeStaged(layout, Staged{})

	require.ErrorIs(t, err, ErrInvalidState)
	assert.Nil(t, data)
}

func TestStaged_Decode_InvalidDocumentsReturnError(t *testing.T) {
	layout, _, _ := newTestState(t)
	testCases := []string{"{", `{"unknown":true}`, `{} {}`, `{}`}

	for _, data := range testCases {
		staged, err := DecodeStaged(layout, []byte(data))

		require.Error(t, err)
		assert.Empty(t, staged)
	}
}

func TestEncode_UnsupportedValueReturnsError(t *testing.T) {
	data, err := encode(func() {})

	require.Error(t, err)
	assert.Nil(t, data)
}
