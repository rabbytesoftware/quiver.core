//go:build integration

package kit

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/rabbytesoftware/quiver/internal/domain"
	"github.com/rabbytesoftware/quiver/internal/engine/vault"
)

// AssertConsistency verifies that API state, vault presence, and List membership
// all agree with the expected values for ns.
//
// wantState="" means expect 404 (removed). Otherwise it's the expected ArrowState.
// wantInList: whether ns should appear in List results.
// wantVault: whether a vault entry should exist for ns.
func AssertConsistency(
	t *testing.T,
	env *Env,
	tc *TypedClient,
	ns string,
	wantState domain.ArrowState,
	wantInList bool,
	wantVault bool,
) {
	t.Helper()

	// 1. Check GetDetail.
	detail, status := tc.GetDetail(ns)
	if wantState == "" {
		if status != http.StatusNotFound {
			t.Errorf("AssertConsistency(%s): GetDetail status = %d, want 404", ns, status)
		}
	} else {
		if status != http.StatusOK {
			t.Errorf("AssertConsistency(%s): GetDetail status = %d, want 200", ns, status)
		} else if detail.State != string(wantState) {
			t.Errorf("AssertConsistency(%s): state = %q, want %q", ns, detail.State, wantState)
		}
	}

	// 2. Check List membership.
	items, _ := tc.List()
	found := false
	for _, item := range items {
		if item.Namespace == ns {
			found = true
			break
		}
	}
	if found != wantInList {
		t.Errorf("AssertConsistency(%s): in list = %v, want %v", ns, found, wantInList)
	}

	// 3. Check vault presence.
	_, err := env.Vault.GetArrow(context.Background(), domain.Namespace(ns))
	inVault := !errors.Is(err, vault.ErrNotCached)
	if inVault != wantVault {
		t.Errorf("AssertConsistency(%s): vault present = %v, want %v", ns, inVault, wantVault)
	}
}
