package arrow

import (
	"context"
	"errors"
	"fmt"

	asynxModels "github.com/char2cs/asynx/models"

	apperrors "github.com/rabbytesoftware/quiver.core/internal/app/errors"
	arrowcmds "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/commands"
	arrowstore "github.com/rabbytesoftware/quiver.core/internal/app/repositories/arrow/internal/store"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

// Advance fetches the manifest at target's commit, replaces the cached one —
// the cache is keyed by the row's identity, which a rolling selector keeps
// across commits — and moves the row to target. A git-only advance has no
// asset checksum, so its fingerprint is the commit.
func (s *arrowService) Advance(
	ctx context.Context,
	ns domain.Namespace,
	target domain.Available,
) error {
	exists, err := s.axArrow.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("advance %s: %w", ns, err)
	}
	if !exists {
		return fmt.Errorf("advance %s: %w", ns, apperrors.ErrNotFound)
	}

	m, raw, filename, err := s.manifold.ResolveArrowAtCommit(ctx, ns, target.Commit)
	if err != nil {
		return fmt.Errorf("advance %s: %w", ns, mapResolveErr(err))
	}

	if err := s.vault.DeleteArrow(ctx, ns); err != nil {
		return fmt.Errorf("advance %s: purge cached manifest: %w", ns, err)
	}
	if err := s.vault.PutArrow(ctx, ns, arrowstore.Cacheable(m, raw, filename)); err != nil {
		return fmt.Errorf("advance %s: cache manifest: %w", ns, err)
	}

	return s.sendAdvance(ctx, ns, m, domain.Resolved{
		Ref:         target.Ref,
		Commit:      target.Commit,
		Fingerprint: target.Commit,
	})
}

// Adopt parses manifest locally, caches it under ns and records resolved as
// what ns has installed: a new user-installed row when ns is absent, an
// advance of the existing row otherwise, and nothing when the row already
// carries resolved.
func (s *arrowService) Adopt(
	ctx context.Context,
	ns domain.Namespace,
	kind domain.SelectorKind,
	resolved domain.Resolved,
	manifest []byte,
) error {
	if ns.Validate() != nil || ns.Ref() == "" {
		return fmt.Errorf("adopt %s: %w", ns, apperrors.ErrInvalidNamespace)
	}

	m, err := s.manifold.ParseArrow(manifest)
	if err != nil {
		return fmt.Errorf("adopt %s: %w: %w", ns, apperrors.ErrInvalidManifest, err)
	}
	if err := s.vault.PutArrow(ctx, ns, arrowstore.Cacheable(m, manifest, "ARROW.md")); err != nil {
		return fmt.Errorf("adopt %s: cache manifest: %w", ns, err)
	}

	exists, err := s.axArrow.Exists(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("adopt %s: %w", ns, err)
	}
	if !exists {
		return s.sendAdopted(ctx, ns, m, kind, resolved)
	}

	current, err := s.axArrow.Get(ctx, ns.String())
	if err != nil {
		return fmt.Errorf("adopt %s: %w", ns, err)
	}
	if current.Resolved == resolved {
		return nil
	}
	return s.sendAdvance(ctx, ns, m, resolved)
}

// sendAdvance waits for the projections: an advance can change the
// dependency edges, and the caller reads them back.
func (s *arrowService) sendAdvance(
	ctx context.Context,
	ns domain.Namespace,
	m *domain.Arrow,
	resolved domain.Resolved,
) error {
	_, err := s.axArrow.SendWait(ctx, arrowcmds.AdvanceArrow{
		Namespace: ns,
		ArrowMeta: m.ArrowMeta,
		Variables: m.Variables,
		Netbridge: m.Netbridge,
		Targets:   m.Targets,
		Readme:    m.Readme,
		Resolved:  resolved,
	})
	return mapSendErr("advance", ns, err)
}

func (s *arrowService) sendAdopted(
	ctx context.Context,
	ns domain.Namespace,
	m *domain.Arrow,
	kind domain.SelectorKind,
	resolved domain.Resolved,
) error {
	_, err := s.axArrow.SendWait(ctx, arrowcmds.AddArrow{
		Namespace:     ns,
		ArrowMeta:     m.ArrowMeta,
		Variables:     m.Variables,
		Netbridge:     m.Netbridge,
		Targets:       m.Targets,
		Readme:        m.Readme,
		DirectInstall: true,
		SelectorKind:  kind,
		Resolved:      resolved,
	})
	return mapSendErr("adopt", ns, err)
}

func mapSendErr(
	op string,
	ns domain.Namespace,
	err error,
) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, asynxModels.ErrValidation) || errors.Is(err, asynxModels.ErrPipelineFailed) {
		return fmt.Errorf("%s %s: %w", op, ns, apperrors.ErrStateViolation)
	}
	return fmt.Errorf("%s %s: %w", op, ns, err)
}
