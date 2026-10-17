package models

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHiddenSceneVisibilities(t *testing.T) {
	withUser := func(role string, permissions ...Permission) context.Context {
		return SetCurrentUser(context.Background(), User{Username: "user", Role: role, Permissions: permissions})
	}
	viewer := withUser(RoleIDViewer, PermissionRecordActivity)

	restrictedAndPrivate := []SceneVisibility{SceneVisibilityRestricted, SceneVisibilityPrivate}

	tests := []struct {
		name string
		ctx  context.Context
		want []SceneVisibility
	}{
		{"no user", context.Background(), nil},
		{"admin", withUser(RoleIDAdmin), nil},
		{"viewer", viewer, restrictedAndPrivate},
		{"editor", withUser(RoleIDEditor, PermissionViewRestrictedScenes), []SceneVisibility{SceneVisibilityPrivate}},
		{"private only", withUser("custom", PermissionViewPrivateScenes), []SceneVisibility{SceneVisibilityRestricted}},
		{"both", withUser("custom", PermissionViewRestrictedScenes, PermissionViewPrivateScenes), nil},
		{"locked user", withUser(""), restrictedAndPrivate},
		{"public only without user", SetPublicScenesOnly(context.Background()), restrictedAndPrivate},
		{"public only over admin", SetPublicScenesOnly(withUser(RoleIDAdmin)), restrictedAndPrivate},
		{"all visible over viewer", SetAllScenesVisible(viewer), nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HiddenSceneVisibilities(tt.ctx))
		})
	}
}
