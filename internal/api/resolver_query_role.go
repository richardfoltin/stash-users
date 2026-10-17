package api

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
)

func (r *queryResolver) Roles(ctx context.Context) ([]*models.Role, error) {
	return r.userService.AllRoles(ctx)
}
