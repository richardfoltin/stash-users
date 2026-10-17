package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/signedurl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withRole returns a context of a user with the role with the ID, who is
// locked if the ID is empty.
func withRole(roleID string) context.Context {
	return models.SetCurrentUser(context.Background(), models.User{Username: "user", Role: roleID})
}

func TestUserAPIKey(t *testing.T) {
	cfg := config.InitializeEmpty()
	cfg.SetString(config.ApiKey, "global-key")

	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{"no current user", context.Background(), "global-key"},
		{"admin", withRole(models.RoleIDAdmin), "global-key"},
		{"editor", withRole(models.RoleIDEditor), ""},
		{"viewer", withRole(models.RoleIDViewer), ""},
		{"locked user", withRole(""), ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, userAPIKey(tt.ctx, cfg))
		})
	}
}

func TestResolveCredentialID(t *testing.T) {
	ctx := context.Background()
	cfg := config.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))
	cfg.SetString(config.JWTSignKey, "test-sign-key")
	require.NoError(t, cfg.UserStore.CreateUser(ctx, models.User{Username: "admin", Role: models.RoleIDAdmin}, "pw"))
	require.NoError(t, cfg.UserStore.CreateUser(ctx, models.User{Username: "viewer", Role: models.RoleIDViewer}, "pw"))
	require.NoError(t, cfg.UserStore.CreateUser(ctx, models.User{Username: "locked"}, "pw"))

	cid := func(username string) string {
		return signedurl.GenerateCredentialID(userSigningKey(cfg, username), username)
	}

	u, secret, found := resolveCredentialID(ctx, cfg, cid("viewer"))
	require.True(t, found)
	assert.Equal(t, "viewer", u.Username)
	assert.Equal(t, []byte("test-sign-key"), secret)

	_, _, found = resolveCredentialID(ctx, cfg, cid("locked"))
	assert.False(t, found, "locked user")
	_, _, found = resolveCredentialID(ctx, cfg, cid("missing"))
	assert.False(t, found, "unknown user")

	// a signed stream URL authenticates as its user
	r := httptest.NewRequest(http.MethodGet, "/scene/1/stream?"+signedParams(cfg, "viewer", signedurl.DerivePrefix("/scene/1/stream")).Encode(), nil)
	u, ok := authenticateSignedRequest(r, true)
	require.True(t, ok)
	assert.Equal(t, "viewer", u.Username)
}
