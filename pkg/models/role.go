package models

import "slices"

// The IDs of the built-in Admin role and the default roles. They are the
// role values that users were stored with before roles were configurable.
const (
	RoleIDAdmin  = "ADMIN"
	RoleIDEditor = "MODIFY"
	RoleIDViewer = "READ"
)

// Role is a named set of permissions.
type Role struct {
	ID          string
	Name        string
	Description string
	Permissions Permissions
}

// IsBuiltin returns true for the Admin role, which has every permission and
// cannot be changed.
func (r Role) IsBuiltin() bool {
	return r.ID == RoleIDAdmin
}

func (r Role) HasPermission(p Permission) bool {
	return r.IsBuiltin() || r.Permissions.Has(p)
}

// AdminRole returns the built-in Admin role.
func AdminRole() Role {
	return Role{
		ID:          RoleIDAdmin,
		Name:        "Admin",
		Description: "Full access, including settings, tasks, deleting and merging objects, and managing users.",
		Permissions: slices.Clone(AllPermissions),
	}
}

// DefaultRoles returns the roles that exist until roles are changed. They
// have the rights of the Editor and Viewer roles before roles were
// configurable.
func DefaultRoles() []Role {
	return []Role{
		{
			ID:          RoleIDEditor,
			Name:        "Editor",
			Description: "Everything a viewer can do, plus creating and editing library metadata and ratings. Cannot delete or merge objects, change settings or run tasks.",
			Permissions: Permissions{
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
			},
		},
		{
			ID:          RoleIDViewer,
			Name:        "Viewer",
			Description: "Can browse and play content, and record their own plays, O counts and resume times. Cannot change library data or settings.",
			Permissions: Permissions{PermissionRecordActivity},
		},
	}
}
