package manifold

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/fletcher"
	"github.com/rabbytesoftware/quiver.core/internal/engine/manifold/resolver"
)

const inferredArrow = "A tool that does things.\n\n" +
	"```arrow\n" +
	`schema: arrow@v0
metadata:
  name: tool
  description: A tool that does things.
  url: https://github.com/acme/tool
  generator:
    name: fletcher/1
    confidence: medium
targets:
  linux/amd64:
    lifecycle:
      install:
        - type: fetch
          title: Download tool-linux-x86_64.tar.gz
          url: https://github.com/acme/tool/releases/download/v1/tool-linux-x86_64.tar.gz
          to: ${INSTALL_PATH}/.tool.download
          checksum: sha256:0000000000000000000000000000000000000000000000000000000000000001
          timeout: 15m
        - type: portable
          title: Install tool-linux-x86_64.tar.gz
          from: ${INSTALL_PATH}/.tool.download
          to: ${INSTALL_PATH}/tool
          name: tool
          timeout: 15m
    expose:
      cli:
        - name: tool
          path: auto
` + "```\n"

type stubFletcher struct {
	raw    []byte
	err    error
	causes []error
}

func (s *stubFletcher) Recover(
	_ context.Context,
	_ domain.Namespace,
	cause error,
) ([]byte, string, error) {
	s.causes = append(s.causes, cause)
	if s.err != nil {
		return nil, "", s.err
	}
	return s.raw, "ARROW.md", nil
}

func withStubFletcher(
	t *testing.T,
	m Manifold,
	fl *stubFletcher,
) Manifold {
	t.Helper()
	built, ok := m.(*manifold)
	require.True(t, ok)
	built.fl = fl
	return built
}

func manifestMissing() error {
	return fmt.Errorf("%w: github.com/acme/tool@v1.2.0", resolver.ErrManifestNotFound)
}

func TestWithFletcher_ReachesEveryConstructor(t *testing.T) {
	testCases := []struct {
		name  string
		build func(opts ...Option) Manifold
	}{
		{
			name: "New",
			build: func(opts ...Option) Manifold {
				return New(time.Second, nil, time.Hour, opts...)
			},
		},
		{
			name: "NewWithClock",
			build: func(opts ...Option) Manifold {
				return NewWithClock(time.Second, nil, time.Hour, time.Now, opts...)
			},
		},
		{
			name: "NewWithResolvers",
			build: func(opts ...Option) Manifold {
				return NewWithResolvers(&stubResolver{}, &stubConstraintResolver{}, nil, opts...)
			},
		},
		{
			name: "NewWithResolversAndClock",
			build: func(opts ...Option) Manifold {
				return NewWithResolversAndClock(&stubResolver{}, &stubConstraintResolver{}, nil, time.Now, opts...)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			enabled, ok := tc.build(WithFletcher(true)).(*manifold)
			require.True(t, ok)
			disabled, ok := tc.build(WithFletcher(false)).(*manifold)
			require.True(t, ok)
			overridden, ok := tc.build(WithFletcher(true), WithFletcher(false)).(*manifold)
			require.True(t, ok)

			assert.NotNil(t, enabled.fl)
			assert.Nil(t, disabled.fl)
			assert.Nil(t, overridden.fl)
		})
	}
}

func TestWithFletcher_ResolveArrow_FallsBackOnlyWhenEnabled(t *testing.T) {
	testCases := []struct {
		name       string
		enabled    bool
		wantReason fletcher.Reason
	}{
		{name: "disabled", enabled: false},
		{name: "enabled", enabled: true, wantReason: fletcher.ReasonHostUnsupported},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewWithResolvers(&stubResolver{arrowErr: manifestMissing()}, &stubConstraintResolver{}, nil, WithFletcher(tc.enabled))

			_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("example.org/acme/tool@v1.0.0"))

			assert.ErrorIs(t, err, resolver.ErrManifestNotFound)
			if tc.wantReason == "" {
				assert.NotErrorIs(t, err, fletcher.ErrNotFletchable)
				return
			}
			var nf fletcher.NotFletchableError
			require.ErrorAs(t, err, &nf)
			assert.Equal(t, tc.wantReason, nf.Reason)
		})
	}
}

func TestResolveArrow_FletcherDisabled_KeepsManifestNotFound(t *testing.T) {
	m := NewWithResolvers(&stubResolver{arrowErr: manifestMissing()}, &stubConstraintResolver{}, nil)

	_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"))

	assert.ErrorIs(t, err, resolver.ErrManifestNotFound)
	assert.ErrorIs(t, err, resolver.ErrNotFound)
	assert.NotErrorIs(t, err, fletcher.ErrNotFletchable)
}

func TestResolveArrow_FletcherEnabled_DeclaredManifestWins(t *testing.T) {
	fl := &stubFletcher{raw: []byte(inferredArrow)}
	declared := []byte(inferredArrow)
	m := withStubFletcher(t, NewWithResolvers(
		&stubResolver{arrowData: declared, arrowFilename: "arrow.yaml"},
		&stubConstraintResolver{},
		nil,
	), fl)

	_, raw, filename, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"))

	require.NoError(t, err)
	assert.Equal(t, declared, raw)
	assert.Equal(t, "arrow.yaml", filename)
	assert.Empty(t, fl.causes)
}

func TestResolveArrow_FletcherEnabled_HandsEveryResolveFailureToFletcher(t *testing.T) {
	testCases := []struct {
		name string
		err  error
	}{
		{name: "manifest not found", err: manifestMissing()},
		{name: "transport failure", err: fmt.Errorf("%w: HTTP 503", resolver.ErrFetchFailed)},
		{name: "bare not found", err: resolver.ErrNotFound},
		{name: "unclassified", err: errors.New("boom")},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fl := &stubFletcher{err: tc.err}
			m := withStubFletcher(t, NewWithResolvers(&stubResolver{arrowErr: tc.err}, &stubConstraintResolver{}, nil), fl)

			_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"))

			assert.ErrorIs(t, err, tc.err)
			require.Len(t, fl.causes, 1)
			assert.ErrorIs(t, fl.causes[0], tc.err)
		})
	}
}

func TestResolveArrow_FletcherEnabled_ManifestNotFound_ForgesInferredArrow(t *testing.T) {
	fl := &stubFletcher{raw: []byte(inferredArrow)}
	m := withStubFletcher(t, NewWithResolvers(&stubResolver{arrowErr: manifestMissing()}, &stubConstraintResolver{}, nil), fl)

	arrow, raw, filename, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@v1.2.0"))

	require.NoError(t, err)
	assert.Equal(t, []byte(inferredArrow), raw)
	assert.Equal(t, "ARROW.md", filename)
	assert.Equal(t, domain.ArrowOriginInferred, arrow.Origin())
	assert.Equal(t, "tool", arrow.Name)
}

func TestResolveArrow_FletcherEnabled_ForgedBytesThatDoNotParse(t *testing.T) {
	fl := &stubFletcher{raw: []byte("not: [a manifest")}
	m := withStubFletcher(t, NewWithResolvers(&stubResolver{arrowErr: manifestMissing()}, &stubConstraintResolver{}, nil), fl)

	_, _, _, err := m.ResolveArrow(context.Background(), domain.Namespace("github.com/acme/tool@main"))

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidManifest)
	assert.NotErrorIs(t, err, resolver.ErrNotFound)
	assert.NotErrorIs(t, err, resolver.ErrFetchFailed)
}
