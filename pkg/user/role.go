package user

import (
	"context"
	"slices"
	"strings"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

// validateRole trims the name of the role, and checks the name and the
// permissions.
func validateRole(r *models.Role) error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return ErrEmptyRoleName
	}

	for i, p := range r.Permissions {
		if !p.IsValid() || slices.Contains(r.Permissions[:i], p) {
			return ErrInvalidPermission
		}
	}

	return nil
}

// AllRoles returns the Admin role, followed by the other roles.
func (s *Service) AllRoles(ctx context.Context) ([]*models.Role, error) {
	return s.Store.AllRoles(ctx)
}

// GetRole returns the role with the ID, or nil if it does not exist.
func (s *Service) GetRole(ctx context.Context, id string) (*models.Role, error) {
	return s.Store.GetRole(ctx, id)
}

// RoleUserCounts returns the number of users of each role that has users.
func (s *Service) RoleUserCounts(ctx context.Context) (map[string]int, error) {
	return s.Store.RoleUserCounts(ctx)
}

func (s *Service) CreateRole(ctx context.Context, r models.Role) (*models.Role, error) {
	if err := validateRole(&r); err != nil {
		return nil, err
	}

	// create role in store - the store assigns the ID and checks that the
	// name is unique
	created, err := s.Store.CreateRole(ctx, r)
	if err != nil {
		return nil, err
	}

	logger.Infof("[role] created %q", created.Name)

	return created, nil
}

func (s *Service) UpdateRole(ctx context.Context, r models.Role) error {
	if err := validateRole(&r); err != nil {
		return err
	}

	// update role in store - the store checks that the role exists, is not
	// the Admin role, and that the name is unique
	if err := s.Store.UpdateRole(ctx, r); err != nil {
		return err
	}

	logger.Infof("[role] updated %q", r.Name)

	return nil
}

func (s *Service) DeleteRole(ctx context.Context, id string) error {
	r, err := s.Store.GetRole(ctx, id)
	if err != nil {
		return err
	}

	if r == nil {
		return ErrRoleNotExist
	}

	// delete role from store - the store checks that it is not the Admin
	// role, and that no user has it
	if err := s.Store.DeleteRole(ctx, id); err != nil {
		return err
	}

	logger.Infof("[role] removed %q", r.Name)

	return nil
}
