package api

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
)

func (r *userResolver) Name(ctx context.Context, obj *models.User) (string, error) {
	return obj.Username, nil
}

func (r *userResolver) Role(ctx context.Context, obj *models.User) (*models.Role, error) {
	if obj.IsLocked() {
		return nil, nil
	}

	return r.userService.GetRole(ctx, obj.Role)
}

func (r *userResolver) Permissions(ctx context.Context, obj *models.User) ([]models.Permission, error) {
	if obj.IsAdmin() {
		return models.AllPermissions, nil
	}

	return obj.Permissions, nil
}
