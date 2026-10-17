package api

import (
	"context"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactConfigResult(t *testing.T) {
	handyKey := "handy-key"
	stashBox := &models.StashBox{Endpoint: "https://stashdb.org/graphql", APIKey: "stash-box-key", Name: "stashdb"}
	c := &ConfigResult{
		General: &ConfigGeneralResult{
			APIKey:     "api-key",
			Username:   "admin",
			Password:   "password-hash",
			StashBoxes: []*models.StashBox{stashBox},
		},
		Interface: &ConfigInterfaceResult{HandyKey: &handyKey},
	}

	redactConfigResult(c)

	assert.Empty(t, c.General.APIKey)
	assert.Empty(t, c.General.Username)
	assert.Empty(t, c.General.Password)
	require.NotNil(t, c.Interface.HandyKey)
	assert.Empty(t, *c.Interface.HandyKey)
	require.Len(t, c.General.StashBoxes, 1)
	assert.Empty(t, c.General.StashBoxes[0].APIKey)
	assert.Equal(t, stashBox.Endpoint, c.General.StashBoxes[0].Endpoint)

	// the redaction does not change the values it copied
	assert.Equal(t, "handy-key", handyKey)
	assert.Equal(t, "stash-box-key", stashBox.APIKey)
}

func TestConfigResultPlugins(t *testing.T) {
	cfg := config.InitializeEmpty()
	cfg.SetPluginConfiguration("plugin", map[string]interface{}{"token": "secret"})
	r := &configResultResolver{&Resolver{}}

	tests := []struct {
		name    string
		ctx     context.Context
		include []string
		want    int
	}{
		{"no current user", context.Background(), nil, 1},
		{"admin", withRoles(models.RoleEnumAdmin), nil, 1},
		{"admin included", withRoles(models.RoleEnumAdmin), []string{"plugin"}, 1},
		{"editor", withRoles(models.RoleEnumModify), nil, 0},
		{"viewer included", withRoles(models.RoleEnumRead), []string{"plugin"}, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := r.Plugins(tt.ctx, &ConfigResult{}, tt.include)
			require.NoError(t, err)
			assert.Len(t, got, tt.want)
		})
	}
}
