package models

import "context"

type contextKey int

const (
	currentUserKey contextKey = iota
	sceneAccessKey
	firstAdminActivityKey
)

func SetCurrentUser(ctx context.Context, u User) context.Context {
	return context.WithValue(ctx, currentUserKey, u)
}

// GetCurrentUser returns the user of the request, or nil if there is none.
func GetCurrentUser(ctx context.Context) *User {
	if u, ok := ctx.Value(currentUserKey).(User); ok {
		return &u
	}

	return nil
}

type sceneAccess int

const (
	allScenesVisible sceneAccess = iota
	publicScenesOnly
)

// SetAllScenesVisible makes every scene visible, regardless of the current
// user.
func SetAllScenesVisible(ctx context.Context) context.Context {
	return context.WithValue(ctx, sceneAccessKey, allScenesVisible)
}

// SetPublicScenesOnly limits the visible scenes to those visible to everyone,
// regardless of the current user.
func SetPublicScenesOnly(ctx context.Context) context.Context {
	return context.WithValue(ctx, sceneAccessKey, publicScenesOnly)
}

// HiddenSceneVisibilities returns the visibilities of the scenes that are
// hidden from the request, or nil if all scenes are visible: for admins and
// when there is no current user.
func HiddenSceneVisibilities(ctx context.Context) []SceneVisibility {
	if access, ok := ctx.Value(sceneAccessKey).(sceneAccess); ok {
		if access == publicScenesOnly {
			return []SceneVisibility{SceneVisibilityRestricted, SceneVisibilityPrivate}
		}
		return nil
	}

	u := GetCurrentUser(ctx)
	if u == nil || u.IsAdmin() {
		return nil
	}

	var ret []SceneVisibility
	if !u.HasPermission(PermissionViewRestrictedScenes) {
		ret = append(ret, SceneVisibilityRestricted)
	}
	if !u.HasPermission(PermissionViewPrivateScenes) {
		ret = append(ret, SceneVisibilityPrivate)
	}

	return ret
}

// WithFirstAdminActivity reads and writes the viewing activity of the first
// admin, whoever the current user is.
func WithFirstAdminActivity(ctx context.Context) context.Context {
	return context.WithValue(ctx, firstAdminActivityKey, true)
}

func IsFirstAdminActivity(ctx context.Context) bool {
	v, _ := ctx.Value(firstAdminActivityKey).(bool)
	return v
}
