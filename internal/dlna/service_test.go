package dlna

import (
	"context"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/models/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRepositoryWithReadTxnVisibility ensures that DLNA clients see only the
// scenes visible to everyone when users are configured.
func TestRepositoryWithReadTxnVisibility(t *testing.T) {
	for _, loginRequired := range []bool{false, true} {
		r := Repository{
			TxnManager:    mocks.NewDatabase(),
			LoginRequired: func(ctx context.Context) bool { return loginRequired },
		}

		var role models.RoleEnum
		var restricted bool
		require.NoError(t, r.WithReadTxn(context.Background(), func(ctx context.Context) error {
			role, restricted = models.ViewerRole(ctx)
			return nil
		}))

		assert.Equal(t, loginRequired, restricted, "restricted when login required is %v", loginRequired)
		if loginRequired {
			assert.Equal(t, models.RoleEnumRead, role)
		}
	}
}
