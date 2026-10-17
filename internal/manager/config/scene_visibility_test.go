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
		want     models.SceneVisibility
	}{
		{"not set", "", models.SceneVisibilityEveryone},
		{"restricted", "default_scene_visibility: RESTRICTED\n", models.SceneVisibilityRestricted},
		{"legacy modify", "default_scene_visibility: MODIFY\n", models.SceneVisibilityRestricted},
		{"legacy admin", "default_scene_visibility: ADMIN\n", models.SceneVisibilityPrivate},
		{"invalid", "default_scene_visibility: UNKNOWN\n", models.SceneVisibilityEveryone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := newTestUserStore(t, tt.contents)
			assert.Equal(t, tt.want, s.Config.GetDefaultSceneVisibility())
		})
	}

	t.Run("set", func(t *testing.T) {
		s, _ := newTestUserStore(t, "")
		s.Config.SetString(DefaultSceneVisibility, models.SceneVisibilityRestricted.String())
		assert.Equal(t, models.SceneVisibilityRestricted, s.Config.GetDefaultSceneVisibility())
	})
}
