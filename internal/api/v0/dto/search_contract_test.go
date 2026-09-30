package dto_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rabbytesoftware/quiver.core/internal/api/v0/dto"
	"github.com/rabbytesoftware/quiver.core/internal/app/models"
	"github.com/rabbytesoftware/quiver.core/internal/app/repositories/discovery"
	"github.com/rabbytesoftware/quiver.core/internal/app/usecases"
	ucmocks "github.com/rabbytesoftware/quiver.core/internal/app/usecases/mocks"
	"github.com/rabbytesoftware/quiver.core/internal/domain"
	"github.com/rabbytesoftware/quiver.core/internal/engine/vault"
	"github.com/rabbytesoftware/quiver.core/internal/mocks"
)

// TestSearchContract_StreamKeyEqualsLaneAKey pins the identity clients merge
// on: the streamed frame and the offline re-query name the same arrow by the
// same bare namespace, whatever ref the pass verified it at.
func TestSearchContract_StreamKeyEqualsLaneAKey(t *testing.T) {
	testCases := []struct {
		name string
		ref  string
	}{
		{"tagged ref", "v1.2.3"},
		{"branch ref", "main"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bare := domain.Namespace("github.com/acme/rocketship")
			meta := domain.ArrowMeta{Name: "rocketship", Description: "launch"}

			streamed := dto.SearchResultDTOFromDiscovery(discovery.Result{
				Namespace: bare,
				Arrow: domain.Arrow{
					Namespace: domain.Namespace(string(bare) + "@" + tc.ref),
					ArrowMeta: meta,
				},
			})

			v := &mocks.Vault{SearchArrowsResult: []vault.IndexRow{{
				Namespace: bare,
				Ref:       tc.ref,
				Meta:      vault.IndexMeta{Arrow: meta},
			}}}
			uc := usecases.NewSearchUsecase(&ucmocks.MockArrow{}, v, &ucmocks.MockCollection{})
			results, err := uc.Search(context.Background(), models.SearchQuery{Text: "rocketship"})
			require.NoError(t, err)
			require.Len(t, results, 1)
			laneA := dto.SearchResultDTOFrom(results[0])

			assert.Equal(t, string(bare), streamed.Namespace)
			assert.Equal(t, streamed.Namespace, laneA.Namespace)
		})
	}
}
