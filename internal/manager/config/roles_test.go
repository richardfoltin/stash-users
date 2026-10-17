package config

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// usersConfig returns a config file with the users and their stored roles.
func usersConfig(t *testing.T, users map[string]string) string {
	t.Helper()

	ret := "users:\n"
	for _, name := range []string{"admin", "editor", "viewer"} {
		if role, ok := users[name]; ok {
			ret += fmt.Sprintf("  - username: %s\n    passwordhash: %s\n    roles: [%s]\n", name, mustHash(t, "pw"), role)
		}
	}
	return ret
}

func roleIDs(t *testing.T, s *UserStore) []string {
	t.Helper()

	roles, err := s.AllRoles(context.Background())
	require.NoError(t, err)

	var ret []string
	for _, r := range roles {
		ret = append(ret, r.ID)
	}
	return ret
}

func TestUserStore_DefaultRolesNotWritten(t *testing.T) {
	contents := usersConfig(t, map[string]string{"admin": "ADMIN", "editor": "MODIFY"})
	s, fn := newTestUserStore(t, contents)

	assert.Equal(t, []string{models.RoleIDAdmin, models.RoleIDEditor, models.RoleIDViewer}, roleIDs(t, s))

	_, err := s.GetUser(context.Background(), "editor")
	require.NoError(t, err)

	data, err := os.ReadFile(fn)
	require.NoError(t, err)
	assert.Equal(t, contents, string(data), "reading roles does not write the config")
}

func TestUserStore_LegacyRoleIDs(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestUserStore(t, usersConfig(t, map[string]string{"admin": "ADMIN", "editor": "MODIFY", "viewer": "READ"}))

	admin, err := s.GetUser(ctx, "admin")
	require.NoError(t, err)
	assert.True(t, admin.IsAdmin())

	defaults := models.DefaultRoles()

	editor, err := s.GetUser(ctx, "editor")
	require.NoError(t, err)
	assert.Equal(t, models.RoleIDEditor, editor.Role)
	assert.ElementsMatch(t, defaults[0].Permissions, editor.Permissions)

	viewer, err := s.GetUser(ctx, "viewer")
	require.NoError(t, err)
	assert.Equal(t, models.RoleIDViewer, viewer.Role)
	assert.ElementsMatch(t, defaults[1].Permissions, viewer.Permissions)
}

// TestUserStore_LegacyMultipleRoles checks that a user stored with several
// roles by an earlier version has the highest of the built-in roles, as then,
// and that the user counts and the removal of roles agree on it.
func TestUserStore_LegacyMultipleRoles(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestUserStore(t, usersConfig(t, map[string]string{"admin": "ADMIN", "editor": "READ, MODIFY"}))

	u, err := s.GetUser(ctx, "editor")
	require.NoError(t, err)
	assert.Equal(t, models.RoleIDEditor, u.Role)

	counts, err := s.RoleUserCounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{models.RoleIDAdmin: 1, models.RoleIDEditor: 1}, counts)

	assert.ErrorIs(t, s.DeleteRole(ctx, models.RoleIDEditor), user.ErrRoleInUse)
	require.NoError(t, s.DeleteRole(ctx, models.RoleIDViewer), "a role that no user has in effect")

	u, err = s.GetUser(ctx, "editor")
	require.NoError(t, err)
	assert.Equal(t, models.RoleIDEditor, u.Role, "unchanged")
}

func TestUserStore_RoleCRUD(t *testing.T) {
	ctx := context.Background()
	s, fn := newTestUserStore(t, usersConfig(t, map[string]string{"admin": "ADMIN", "editor": "MODIFY"}))

	created, err := s.CreateRole(ctx, models.Role{
		Name:        "Marker editor",
		Description: "Edits markers",
		Permissions: models.Permissions{models.PermissionMarkersEdit, models.PermissionMarkersDelete},
	})
	require.NoError(t, err)
	assert.Len(t, created.ID, 16)
	assert.Equal(t, "Marker editor", created.Name)

	got, err := s.GetRole(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created, got)

	// the defaults are written along with the first change
	assert.Equal(t, []string{models.RoleIDAdmin, models.RoleIDEditor, models.RoleIDViewer, created.ID}, roleIDs(t, loadTestUserStore(t, fn)))

	require.NoError(t, s.ReplaceUser(ctx, "editor", models.User{Username: "editor", Role: created.ID}))
	u, err := s.GetUser(ctx, "editor")
	require.NoError(t, err)
	assert.ElementsMatch(t, models.Permissions{models.PermissionMarkersEdit, models.PermissionMarkersDelete}, u.Permissions)

	// a changed role applies to its users at once
	created.Permissions = models.Permissions{models.PermissionMarkersEdit}
	require.NoError(t, s.UpdateRole(ctx, *created))
	u, err = s.GetUser(ctx, "editor")
	require.NoError(t, err)
	assert.Equal(t, models.Permissions{models.PermissionMarkersEdit}, u.Permissions)

	counts, err := s.RoleUserCounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{models.RoleIDAdmin: 1, created.ID: 1}, counts)

	// a role in use cannot be removed
	assert.ErrorIs(t, s.DeleteRole(ctx, created.ID), user.ErrRoleInUse)
	require.NoError(t, s.ReplaceUser(ctx, "editor", models.User{Username: "editor", Role: models.RoleIDViewer}))
	require.NoError(t, s.DeleteRole(ctx, created.ID))
	assert.Equal(t, []string{models.RoleIDAdmin, models.RoleIDEditor, models.RoleIDViewer}, roleIDs(t, loadTestUserStore(t, fn)))

	assert.ErrorIs(t, s.DeleteRole(ctx, created.ID), user.ErrRoleNotExist)
	assert.ErrorIs(t, s.UpdateRole(ctx, *created), user.ErrRoleNotExist)
}

func TestUserStore_DeleteAllRoles(t *testing.T) {
	ctx := context.Background()
	s, fn := newTestUserStore(t, usersConfig(t, map[string]string{"admin": "ADMIN"}))

	require.NoError(t, s.DeleteRole(ctx, models.RoleIDEditor))
	require.NoError(t, s.DeleteRole(ctx, models.RoleIDViewer))

	// an empty list is kept, rather than bringing back the default roles
	assert.Equal(t, []string{models.RoleIDAdmin}, roleIDs(t, s))
	assert.Equal(t, []string{models.RoleIDAdmin}, roleIDs(t, loadTestUserStore(t, fn)))
}

func TestUserStore_BuiltinRole(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestUserStore(t, usersConfig(t, map[string]string{"admin": "ADMIN"}))

	admin, err := s.GetRole(ctx, models.RoleIDAdmin)
	require.NoError(t, err)
	assert.True(t, admin.IsBuiltin())

	assert.ErrorIs(t, s.UpdateRole(ctx, models.Role{ID: models.RoleIDAdmin, Name: "Boss"}), user.ErrBuiltinRole)
	assert.ErrorIs(t, s.DeleteRole(ctx, models.RoleIDAdmin), user.ErrBuiltinRole)
}

func TestUserStore_RoleNameUnique(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestUserStore(t, usersConfig(t, map[string]string{"admin": "ADMIN"}))

	for _, name := range []string{"Editor", "editor", " EDITOR ", "admin"} {
		_, err := s.CreateRole(ctx, models.Role{Name: name})
		assert.ErrorIs(t, err, user.ErrRoleNameExists, name)
	}

	// a role keeps its own name
	editor, err := s.GetRole(ctx, models.RoleIDEditor)
	require.NoError(t, err)
	editor.Name = "EDITOR"
	assert.NoError(t, s.UpdateRole(ctx, *editor))

	editor.Name = "Viewer"
	assert.ErrorIs(t, s.UpdateRole(ctx, *editor), user.ErrRoleNameExists)
}

func TestUserStore_UnknownRole(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestUserStore(t, usersConfig(t, map[string]string{"admin": "ADMIN"}))

	assert.ErrorIs(t, s.CreateUser(ctx, models.User{Username: "x", Role: "nope"}, "pw"), user.ErrRoleNotExist)
	assert.ErrorIs(t, s.ReplaceUser(ctx, "admin", models.User{Username: "admin", Role: "nope"}), user.ErrRoleNotExist)

	// a user stored with a role that no longer exists is locked
	s2, _ := newTestUserStore(t, usersConfig(t, map[string]string{"admin": "ADMIN", "viewer": "gone"}))
	u, err := s2.GetUser(ctx, "viewer")
	require.NoError(t, err)
	assert.True(t, u.IsLocked())
}

func TestUserStore_StoredPermissions(t *testing.T) {
	ctx := context.Background()
	contents := usersConfig(t, map[string]string{"admin": "ADMIN", "viewer": "custom"}) +
		"roles:\n  - id: custom\n    name: Custom\n    permissions: [TAGS_EDIT, NOPE]\n"
	s, _ := newTestUserStore(t, contents)

	// unknown permissions in the config file are ignored
	role, err := s.GetRole(ctx, "custom")
	require.NoError(t, err)
	assert.Equal(t, models.Permissions{models.PermissionTagsEdit}, role.Permissions)

	u, err := s.GetUser(ctx, "viewer")
	require.NoError(t, err)
	assert.Equal(t, models.Permissions{models.PermissionTagsEdit}, u.Permissions)
}

// TestUserStore_RoleConcurrency removes a role while it is concurrently given
// to a user: the user never ends up with a removed role.
func TestUserStore_RoleConcurrency(t *testing.T) {
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		s, fn := newTestUserStore(t, usersConfig(t, map[string]string{"admin": "ADMIN", "viewer": "READ"}))
		role, err := s.CreateRole(ctx, models.Role{Name: "Temp"})
		require.NoError(t, err)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = s.DeleteRole(ctx, role.ID)
		}()
		go func() {
			defer wg.Done()
			_ = s.ReplaceUser(ctx, "viewer", models.User{Username: "viewer", Role: role.ID})
		}()
		wg.Wait()

		reloaded := loadTestUserStore(t, fn)
		u, err := reloaded.GetUser(ctx, "viewer")
		require.NoError(t, err)
		if u.Role == role.ID {
			got, err := reloaded.GetRole(ctx, role.ID)
			require.NoError(t, err)
			assert.NotNil(t, got, "the role of the user exists")
		}
	}
}
