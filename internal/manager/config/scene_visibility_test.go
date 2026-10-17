package config

import (
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestGetDefaultSceneVisibility(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     models.RoleEnum
	}{
		{"not set", "", models.RoleEnumRead},
		{"modify", "default_scene_visibility: MODIFY\n", models.RoleEnumModify},
		{"admin", "default_scene_visibility: ADMIN\n", models.RoleEnumAdmin},
		{"invalid", "default_scene_visibility: UNKNOWN\n", models.RoleEnumRead},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newTestUserStore(t, tt.contents)
			assert.Equal(t, tt.want, s.Config.GetDefaultSceneVisibility())
		})
	}

	t.Run("set", func(t *testing.T) {
		s, _ := newTestUserStore(t, "")
		s.Config.SetString(DefaultSceneVisibility, models.RoleEnumModify.String())
		assert.Equal(t, models.RoleEnumModify, s.Config.GetDefaultSceneVisibility())
	})
}
