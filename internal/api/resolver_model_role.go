package api

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
)

func (r *roleResolver) Builtin(ctx context.Context, obj *models.Role) (bool, error) {
	return obj.IsBuiltin(), nil
}

func (r *roleResolver) Permissions(ctx context.Context, obj *models.Role) ([]models.Permission, error) {
	return obj.Permissions, nil
}

func (r *roleResolver) UserCount(ctx context.Context, obj *models.Role) (int, error) {
	// the counts are read from the config file in memory, so they are not
	// batched for the few roles
	counts, err := r.userService.RoleUserCounts(ctx)
	if err != nil {
		return 0, err
	}

	return counts[obj.ID], nil
}
