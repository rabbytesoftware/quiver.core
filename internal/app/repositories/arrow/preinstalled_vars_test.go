package arrow

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rabbytesoftware/quiver.core/internal/domain"
)

func TestPreinstalledVars_Ref(t *testing.T) {
	testCases := []struct {
		name     string
		ns       domain.Namespace
		resolved domain.Resolved
		wantRef  string
	}{
		{name: "channel identity takes the resolved ref", ns: "github.com/u/r@stable", resolved: domain.Resolved{Ref: "v1.2.0"}, wantRef: "v1.2.0"},
		{name: "no resolved state falls back to identity", ns: "github.com/u/r@v1.0.0", wantRef: "v1.0.0"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vars := preinstalledVars(tc.ns, &domain.Arrow{Resolved: tc.resolved}, domain.OSLinuxAMD64)
			assert.Equal(t, tc.wantRef, vars[domain.VarRef])
		})
	}
}
