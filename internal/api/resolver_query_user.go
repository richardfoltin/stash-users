package api

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
)

func (r *queryResolver) Users(ctx context.Context) ([]*models.User, error) {
	return r.userService.AllUsers(ctx)
}

func (r *queryResolver) Me(ctx context.Context) (*models.User, error) {
	// get current user
	return models.GetCurrentUser(ctx), nil
}
