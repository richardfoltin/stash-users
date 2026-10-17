package models

import (
	"fmt"
	"io"
	"slices"
	"strconv"
)

// Permission is a right that a role can grant.
type Permission string

const (
	PermissionScenesEdit       Permission = "SCENES_EDIT"
	PermissionScenesDelete     Permission = "SCENES_DELETE"
	PermissionScenesMerge      Permission = "SCENES_MERGE"
	PermissionMarkersEdit      Permission = "MARKERS_EDIT"
	PermissionMarkersDelete    Permission = "MARKERS_DELETE"
	PermissionImagesEdit       Permission = "IMAGES_EDIT"
	PermissionImagesDelete     Permission = "IMAGES_DELETE"
	PermissionGalleriesEdit    Permission = "GALLERIES_EDIT"
	PermissionGalleriesDelete  Permission = "GALLERIES_DELETE"
	PermissionChaptersEdit     Permission = "CHAPTERS_EDIT"
	PermissionChaptersDelete   Permission = "CHAPTERS_DELETE"
	PermissionPerformersEdit   Permission = "PERFORMERS_EDIT"
	PermissionPerformersDelete Permission = "PERFORMERS_DELETE"
	PermissionPerformersMerge  Permission = "PERFORMERS_MERGE"
	PermissionStudiosEdit      Permission = "STUDIOS_EDIT"
	PermissionStudiosDelete    Permission = "STUDIOS_DELETE"
	PermissionTagsEdit         Permission = "TAGS_EDIT"
	PermissionTagsDelete       Permission = "TAGS_DELETE"
	PermissionTagsMerge        Permission = "TAGS_MERGE"
	PermissionGroupsEdit       Permission = "GROUPS_EDIT"
	PermissionGroupsDelete     Permission = "GROUPS_DELETE"

	PermissionRecordActivity           Permission = "RECORD_ACTIVITY"
	PermissionUseScrapers              Permission = "USE_SCRAPERS"
	PermissionSaveFilters              Permission = "SAVE_FILTERS"
	PermissionDeleteSavedFilters       Permission = "DELETE_SAVED_FILTERS"
	PermissionManageFiles              Permission = "MANAGE_FILES"
	PermissionViewRestrictedScenes     Permission = "VIEW_RESTRICTED_SCENES"
	PermissionViewPrivateScenes        Permission = "VIEW_PRIVATE_SCENES"
	PermissionChangeSceneVisibility    Permission = "CHANGE_SCENE_VISIBILITY"
	PermissionRunLibraryTasks          Permission = "RUN_LIBRARY_TASKS"
	PermissionManageSharedListSettings Permission = "MANAGE_SHARED_LIST_SETTINGS"
)

// AllPermissions lists the permissions in the order they are shown.
var AllPermissions = []Permission{
	PermissionScenesEdit,
	PermissionScenesDelete,
	PermissionScenesMerge,
	PermissionMarkersEdit,
	PermissionMarkersDelete,
	PermissionImagesEdit,
	PermissionImagesDelete,
	PermissionGalleriesEdit,
	PermissionGalleriesDelete,
	PermissionChaptersEdit,
	PermissionChaptersDelete,
	PermissionPerformersEdit,
	PermissionPerformersDelete,
	PermissionPerformersMerge,
	PermissionStudiosEdit,
	PermissionStudiosDelete,
	PermissionTagsEdit,
	PermissionTagsDelete,
	PermissionTagsMerge,
	PermissionGroupsEdit,
	PermissionGroupsDelete,
	PermissionRecordActivity,
	PermissionUseScrapers,
	PermissionSaveFilters,
	PermissionDeleteSavedFilters,
	PermissionManageFiles,
	PermissionViewRestrictedScenes,
	PermissionViewPrivateScenes,
	PermissionChangeSceneVisibility,
	PermissionRunLibraryTasks,
	PermissionManageSharedListSettings,
}

func (p Permission) IsValid() bool {
	return slices.Contains(AllPermissions, p)
}

func (p Permission) String() string {
	return string(p)
}

func (p *Permission) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*p = Permission(str)
	if !p.IsValid() {
		return fmt.Errorf("%s is not a valid Permission", str)
	}
	return nil
}

func (p Permission) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(p.String()))
}

type Permissions []Permission

func (ps Permissions) Has(p Permission) bool {
	return slices.Contains(ps, p)
}
