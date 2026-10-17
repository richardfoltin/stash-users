package models

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultRoles(t *testing.T) {
	roles := DefaultRoles()
	assert.Len(t, roles, 2)

	editor, viewer := roles[0], roles[1]
	assert.Equal(t, RoleIDEditor, editor.ID)
	assert.Equal(t, "Editor", editor.Name)
	assert.Equal(t, RoleIDViewer, viewer.ID)
	assert.Equal(t, "Viewer", viewer.Name)

	assert.ElementsMatch(t, Permissions{
		PermissionScenesEdit,
		PermissionMarkersEdit,
		PermissionImagesEdit,
		PermissionGalleriesEdit,
		PermissionChaptersEdit,
		PermissionPerformersEdit,
		PermissionStudiosEdit,
		PermissionTagsEdit,
		PermissionGroupsEdit,
		PermissionRecordActivity,
		PermissionUseScrapers,
		PermissionSaveFilters,
		PermissionViewRestrictedScenes,
	}, editor.Permissions)
	assert.Equal(t, Permissions{PermissionRecordActivity}, viewer.Permissions)

	// the default roles have none of the rights that were admin-only
	adminOnly := []Permission{
		PermissionManageFiles,
		PermissionViewPrivateScenes,
		PermissionChangeSceneVisibility,
		PermissionRunLibraryTasks,
		PermissionManageSharedListSettings,
		PermissionDeleteSavedFilters,
	}
	for _, r := range roles {
		for _, p := range r.Permissions {
			assert.False(t, strings.HasSuffix(p.String(), "_DELETE") || strings.HasSuffix(p.String(), "_MERGE"), "%s has %s", r.Name, p)
			assert.NotContains(t, adminOnly, p, "%s has %s", r.Name, p)
		}
	}
}

func TestAllPermissions(t *testing.T) {
	assert.Len(t, AllPermissions, 31)

	for _, p := range AllPermissions {
		assert.True(t, p.IsValid(), p)
	}

	assert.False(t, Permission("NOPE").IsValid())
}

func TestAdminRole(t *testing.T) {
	admin := AdminRole()
	assert.Equal(t, RoleIDAdmin, admin.ID)
	assert.True(t, admin.IsBuiltin())

	for _, p := range AllPermissions {
		assert.True(t, admin.HasPermission(p), p)
	}

	for _, r := range DefaultRoles() {
		assert.False(t, r.IsBuiltin(), r.Name)
	}
}

func TestUserHasPermission(t *testing.T) {
	admin := User{Username: "admin", Role: RoleIDAdmin}
	for _, p := range AllPermissions {
		assert.True(t, admin.HasPermission(p), p)
	}
	assert.True(t, admin.IsAdmin())

	locked := User{Username: "locked"}
	assert.True(t, locked.IsLocked())
	for _, p := range AllPermissions {
		assert.False(t, locked.HasPermission(p), p)
	}

	editor := User{Username: "editor", Role: RoleIDEditor, Permissions: DefaultRoles()[0].Permissions}
	assert.False(t, editor.IsAdmin())
	assert.False(t, editor.IsLocked())
	assert.True(t, editor.HasPermission(PermissionTagsEdit))
	assert.False(t, editor.HasPermission(PermissionTagsDelete))
}
