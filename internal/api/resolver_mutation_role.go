package api

import (
	"context"

	"github.com/stashapp/stash/pkg/models"
)

// stringValue returns the string, or an empty string if it is nil.
func stringValue(s *string) string {
	if s == nil {
		return ""
	}

	return *s
}

func (r *mutationResolver) RoleCreate(ctx context.Context, input RoleCreateInput) (*models.Role, error) {
	return r.userService.CreateRole(ctx, models.Role{
		Name:        input.Name,
		Description: stringValue(input.Description),
		Permissions: input.Permissions,
	})
}

func (r *mutationResolver) RoleUpdate(ctx context.Context, input RoleUpdateInput) (*models.Role, error) {
	err := r.userService.UpdateRole(ctx, models.Role{
		ID:          input.ID,
		Name:        input.Name,
		Description: stringValue(input.Description),
		Permissions: input.Permissions,
	})
	if err != nil {
		return nil, err
	}

	return r.userService.GetRole(ctx, input.ID)
}

func (r *mutationResolver) RoleDestroy(ctx context.Context, input RoleDestroyInput) (bool, error) {
	if err := r.userService.DeleteRole(ctx, input.ID); err != nil {
		return false, err
	}

	return true, nil
}
