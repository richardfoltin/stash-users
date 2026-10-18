package config

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/user"
)

const Roles = "roles"

// StoredRole is a role as stored in the config file. The built-in Admin role
// is not stored.
type StoredRole struct {
	ID          string              `json:"id" koanf:"id"`
	Name        string              `json:"name" koanf:"name"`
	Description string              `json:"description" koanf:"description"`
	Permissions []models.Permission `json:"permissions" koanf:"permissions"`
}

func newStoredRole(r models.Role) StoredRole {
	return StoredRole{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Permissions: slices.Clone(r.Permissions),
	}
}

func (r StoredRole) toMap() map[string]interface{} {
	permissions := make([]string, len(r.Permissions))
	for i, p := range r.Permissions {
		permissions[i] = p.String()
	}

	return map[string]interface{}{
		"id":          r.ID,
		"name":        r.Name,
		"description": r.Description,
		"permissions": permissions,
	}
}

// validPermissions returns the permissions of the role that exist. Unknown
// permissions in the config file are ignored.
func (r StoredRole) validPermissions() models.Permissions {
	var ret models.Permissions
	for _, p := range r.Permissions {
		if p.IsValid() {
			ret = append(ret, p)
		}
	}
	return ret
}

func (r StoredRole) toRole() *models.Role {
	return &models.Role{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		Permissions: r.validPermissions(),
	}
}

func newRoleID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating role id: %w", err)
	}

	return hex.EncodeToString(b), nil
}

func findRole(roles []StoredRole, id string) int {
	return slices.IndexFunc(roles, func(r StoredRole) bool {
		return r.ID == id
	})
}

// roleNameTaken returns true if a role other than the one with the ID has the
// name, ignoring case and surrounding whitespace. The Admin role counts.
func roleNameTaken(roles []StoredRole, id string, name string) bool {
	name = strings.TrimSpace(name)
	if strings.EqualFold(name, models.AdminRole().Name) {
		return true
	}

	return slices.ContainsFunc(roles, func(r StoredRole) bool {
		return r.ID != id && strings.EqualFold(strings.TrimSpace(r.Name), name)
	})
}

// rolesNoLock returns the roles other than Admin: the stored roles, or the
// default roles if roles were never changed. Every request reads them, so they
// are decoded once for each change of the configuration, and the roles are
// shared: they must not be changed. Assumes read lock held.
func (s *UserStore) rolesNoLock() ([]StoredRole, error) {
	if c := s.roles.Load(); c != nil && c.generation == s.generation {
		return c.value, nil
	}

	var ret []StoredRole
	if !s.forKey(Roles).Exists(Roles) {
		defaults := models.DefaultRoles()
		ret = make([]StoredRole, len(defaults))
		for i, r := range defaults {
			ret[i] = newStoredRole(r)
		}
	} else if err := s.forKey(Roles).Unmarshal(Roles, &ret); err != nil {
		return nil, fmt.Errorf("reading roles from config: %w", err)
	}

	s.roles.Store(&decoded[[]StoredRole]{generation: s.generation, value: ret})
	return ret, nil
}

// AllRoles returns the Admin role, followed by the other roles.
func (s *UserStore) AllRoles(ctx context.Context) ([]*models.Role, error) {
	s.RLock()
	defer s.RUnlock()

	roles, err := s.rolesNoLock()
	if err != nil {
		return nil, err
	}

	admin := models.AdminRole()
	ret := []*models.Role{&admin}
	for _, r := range roles {
		ret = append(ret, r.toRole())
	}

	return ret, nil
}

// GetRole returns the role with the ID, or nil if not found.
func (s *UserStore) GetRole(ctx context.Context, id string) (*models.Role, error) {
	if id == models.RoleIDAdmin {
		admin := models.AdminRole()
		return &admin, nil
	}

	s.RLock()
	defer s.RUnlock()

	roles, err := s.rolesNoLock()
	if err != nil {
		return nil, err
	}

	idx := findRole(roles, id)
	if idx == -1 {
		return nil, nil
	}

	return roles[idx].toRole(), nil
}

// RoleUserCounts returns the number of users of each role that has users.
func (s *UserStore) RoleUserCounts(ctx context.Context) (map[string]int, error) {
	s.RLock()
	defer s.RUnlock()

	users, err := s.allUsersNoLock()
	if err != nil {
		return nil, err
	}

	roles, err := s.rolesNoLock()
	if err != nil {
		return nil, err
	}

	ret := make(map[string]int)
	for _, u := range users {
		if role := u.toUser(roles).Role; role != "" {
			ret[role]++
		}
	}

	return ret, nil
}

// modifyRoles applies fn to the roles and writes the config file. The write
// lock is held throughout, so fn can check the users atomically.
func (s *UserStore) modifyRoles(fn func(roles []StoredRole, users []StoredUser) ([]StoredRole, error)) error {
	s.Lock()
	defer s.Unlock()

	roles, err := s.rolesNoLock()
	if err != nil {
		return err
	}

	users, err := s.allUsersNoLock()
	if err != nil {
		return err
	}

	// fn changes the roles, which must not be the shared ones
	roles, err = fn(slices.Clone(roles), users)
	if err != nil {
		return err
	}

	// an empty list is written too, so that the default roles do not return
	stored := make([]map[string]interface{}, len(roles))
	for i, r := range roles {
		stored[i] = r.toMap()
	}
	s.set(Roles, stored)

	return s.writeNoLock()
}

// CreateRole adds a role with a new ID, and returns it.
func (s *UserStore) CreateRole(ctx context.Context, r models.Role) (*models.Role, error) {
	id, err := newRoleID()
	if err != nil {
		return nil, err
	}

	created := newStoredRole(r)
	created.ID = id

	if err := s.modifyRoles(func(roles []StoredRole, users []StoredUser) ([]StoredRole, error) {
		if roleNameTaken(roles, id, r.Name) {
			return nil, user.ErrRoleNameExists
		}

		return append(roles, created), nil
	}); err != nil {
		return nil, err
	}

	return created.toRole(), nil
}

// UpdateRole replaces the name, description and permissions of the role.
func (s *UserStore) UpdateRole(ctx context.Context, r models.Role) error {
	if r.IsBuiltin() {
		return user.ErrBuiltinRole
	}

	return s.modifyRoles(func(roles []StoredRole, users []StoredUser) ([]StoredRole, error) {
		idx := findRole(roles, r.ID)
		if idx == -1 {
			return nil, user.ErrRoleNotExist
		}

		if roleNameTaken(roles, r.ID, r.Name) {
			return nil, user.ErrRoleNameExists
		}

		roles[idx] = newStoredRole(r)
		return roles, nil
	})
}

// DeleteRole removes the role. A role that users have cannot be removed: the
// role in effect, as the user counts count them.
func (s *UserStore) DeleteRole(ctx context.Context, id string) error {
	if id == models.RoleIDAdmin {
		return user.ErrBuiltinRole
	}

	return s.modifyRoles(func(roles []StoredRole, users []StoredUser) ([]StoredRole, error) {
		idx := findRole(roles, id)
		if idx == -1 {
			return nil, user.ErrRoleNotExist
		}

		if slices.ContainsFunc(users, func(u StoredUser) bool {
			return u.toUser(roles).Role == id
		}) {
			return nil, user.ErrRoleInUse
		}

		return slices.Delete(roles, idx, idx+1), nil
	})
}
