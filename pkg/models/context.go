package models

import "context"

type contextKey int

const (
	currentUserKey contextKey = iota
	viewerRoleKey
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

// SetViewerRole limits the visible content to that of the role, regardless of
// the current user. The ADMIN role lifts the limit.
func SetViewerRole(ctx context.Context, role RoleEnum) context.Context {
	return context.WithValue(ctx, viewerRoleKey, role)
}

// ViewerRole returns the role that limits the visible content, and false if
// all content is visible: for admins and when there is no current user.
func ViewerRole(ctx context.Context) (RoleEnum, bool) {
	if role, ok := ctx.Value(viewerRoleKey).(RoleEnum); ok {
		return role, role != RoleEnumAdmin
	}

	u := GetCurrentUser(ctx)
	switch {
	case u == nil || u.IsAdmin():
		return "", false
	case u.Roles.HasRole(RoleEnumModify):
		return RoleEnumModify, true
	default:
		return RoleEnumRead, true
	}
}
