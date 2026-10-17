package config_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	adminRole  = models.RoleIDAdmin
	modifyRole = models.RoleIDEditor
	readRole   = models.RoleIDViewer
)

// newTestService returns a user service backed by an empty config file.
func newTestService(t *testing.T) (*user.Service, *config.Config) {
	t.Helper()

	cfg := config.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))

	return &user.Service{Store: cfg.UserStore, APIKeyUserID: manager.GetUserIDFromAPIKey}, cfg
}

func TestUserService_FirstUserMustBeAdmin(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)

	assert.False(t, s.LoginRequired(ctx))
	assert.ErrorIs(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: readRole}, "pw"), user.ErrFirstUserNotAdmin)
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: adminRole}, "pw"))
	assert.True(t, s.LoginRequired(ctx))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: readRole}, "pw"))
}

func TestUserService_CreateValidation(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)

	tests := []struct {
		name     string
		user     models.User
		password string
		want     error
	}{
		{"empty username", models.User{Username: "", Role: adminRole}, "pw", user.ErrEmptyUsername},
		{"username with whitespace", models.User{Username: " admin", Role: adminRole}, "pw", user.ErrUsernameHasWhitespace},
		{"empty password", models.User{Username: "admin", Role: adminRole}, "", user.ErrEmptyPassword},
		{"unknown role", models.User{Username: "admin", Role: "SUPERUSER"}, "pw", user.ErrRoleNotExist},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, s.CreateUser(ctx, tt.user, tt.password), tt.want)
		})
	}

	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: adminRole}, "pw"))
	assert.ErrorIs(t, s.CreateUser(ctx, models.User{Username: "admin", Role: adminRole}, "pw"), user.ErrUserAlreadyExists)
}

func TestUserService_LastAdminProtection(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)

	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: adminRole}, "pw"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "editor", Role: modifyRole}, "pw"))

	// the last admin cannot be demoted or removed - even if it is the only user
	assert.ErrorIs(t, s.UpdateUser(ctx, "admin", models.User{Username: "admin", Role: modifyRole}), user.ErrRemoveLastAdminRole)
	assert.ErrorIs(t, s.UpdateUser(ctx, "admin", models.User{Username: "admin"}), user.ErrRemoveLastAdminRole)
	assert.ErrorIs(t, s.DeleteUser(ctx, "admin"), user.ErrDeleteLastAdminUser)

	// renaming the last admin is allowed
	require.NoError(t, s.UpdateUser(ctx, "admin", models.User{Username: "root", Role: adminRole}))

	// non-admins can be removed
	require.NoError(t, s.DeleteUser(ctx, "editor"))
	assert.ErrorIs(t, s.DeleteUser(ctx, "root"), user.ErrDeleteLastAdminUser)

	// with a second admin, either admin can be demoted or removed
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin2", Role: adminRole}, "pw"))
	require.NoError(t, s.UpdateUser(ctx, "root", models.User{Username: "root", Role: readRole}))
	assert.ErrorIs(t, s.DeleteUser(ctx, "admin2"), user.ErrDeleteLastAdminUser)
	require.NoError(t, s.UpdateUser(ctx, "root", models.User{Username: "root", Role: adminRole}))
	require.NoError(t, s.DeleteUser(ctx, "admin2"))

	users, err := s.AllUsers(ctx)
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "root", users[0].Username)
	assert.Equal(t, adminRole, users[0].Role)

	assert.ErrorIs(t, s.UpdateUser(ctx, "missing", models.User{Username: "missing", Role: readRole}), user.ErrUserNotExist)
	assert.ErrorIs(t, s.DeleteUser(ctx, "missing"), user.ErrUserNotExist)
}

func TestUserService_LegacyUserIsAdmin(t *testing.T) {
	ctx := context.Background()
	s, cfg := newTestService(t)

	// existing single-user configuration
	username := "legacy"
	password := "secret"
	require.NoError(t, cfg.UserStore.SetLegacyCredentials(&username, &password))

	assert.True(t, s.LoginRequired(ctx))
	assert.NoError(t, s.ValidateCredentials(ctx, "legacy", "secret"))
	assert.ErrorIs(t, s.ValidateCredentials(ctx, "legacy", "wrong"), user.ErrAccessDenied)

	token, err := s.Login(ctx, "legacy", "secret")
	require.NoError(t, err)
	u, err := s.AuthenticateSession(ctx, "legacy", token)
	require.NoError(t, err)
	assert.Equal(t, adminRole, u.Role)

	// the legacy user is the last admin
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: readRole}, "pw"))
	assert.ErrorIs(t, s.DeleteUser(ctx, "legacy"), user.ErrDeleteLastAdminUser)
	assert.ErrorIs(t, s.UpdateUser(ctx, "legacy", models.User{Username: "legacy", Role: readRole}), user.ErrRemoveLastAdminRole)

	// and can still log in after being moved into the users list, keeping
	// existing sessions
	assert.NoError(t, s.ValidateCredentials(ctx, "legacy", "secret"))
	_, err = s.AuthenticateSession(ctx, "legacy", token)
	assert.NoError(t, err)
}

func TestUserService_Authentication(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)

	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: adminRole}, "adminpw"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: readRole}, "viewerpw"))

	assert.NoError(t, s.ValidateCredentials(ctx, "viewer", "viewerpw"))
	assert.ErrorIs(t, s.ValidateCredentials(ctx, "viewer", "adminpw"), user.ErrAccessDenied)
	assert.ErrorIs(t, s.ValidateCredentials(ctx, "missing", "pw"), user.ErrAccessDenied)

	_, err := s.Login(ctx, "viewer", "adminpw")
	assert.ErrorIs(t, err, user.ErrAccessDenied)
	viewerToken, err := s.Login(ctx, "viewer", "viewerpw")
	require.NoError(t, err)

	u, err := s.AuthenticateSession(ctx, "viewer", viewerToken)
	require.NoError(t, err)
	assert.Equal(t, readRole, u.Role)
	_, err = s.AuthenticateSession(ctx, "viewer", "")
	assert.ErrorIs(t, err, user.ErrAccessDenied, "session without token")
	_, err = s.AuthenticateSession(ctx, "viewer", "wrong")
	assert.ErrorIs(t, err, user.ErrAccessDenied)

	// a user without roles is locked
	require.NoError(t, s.UpdateUser(ctx, "viewer", models.User{Username: "viewer"}))
	assert.ErrorIs(t, s.ValidateCredentials(ctx, "viewer", "viewerpw"), user.ErrAccessDenied)
	_, err = s.AuthenticateSession(ctx, "viewer", viewerToken)
	assert.ErrorIs(t, err, user.ErrAccessDenied)

	// deleted users cannot authenticate
	require.NoError(t, s.DeleteUser(ctx, "viewer"))
	_, err = s.AuthenticateSession(ctx, "viewer", viewerToken)
	assert.ErrorIs(t, err, user.ErrAccessDenied)

	// a user created again with the same name does not get the sessions of
	// the deleted user
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: readRole}, "viewerpw"))
	_, err = s.AuthenticateSession(ctx, "viewer", viewerToken)
	assert.ErrorIs(t, err, user.ErrAccessDenied)

	// self-service password change requires the current password, and ends
	// the existing sessions
	adminToken, err := s.Login(ctx, "admin", "adminpw")
	require.NoError(t, err)
	assert.ErrorIs(t, s.ChangePassword(ctx, "admin", "wrong", "new"), user.ErrCurrentPasswordIncorrect)
	require.NoError(t, s.ChangePassword(ctx, "admin", "adminpw", "new"))
	assert.NoError(t, s.ValidateCredentials(ctx, "admin", "new"))
	_, err = s.AuthenticateSession(ctx, "admin", adminToken)
	assert.ErrorIs(t, err, user.ErrAccessDenied)
	adminToken, err = s.Login(ctx, "admin", "new")
	require.NoError(t, err)
	_, err = s.AuthenticateSession(ctx, "admin", adminToken)
	assert.NoError(t, err)

	// an administrator setting the password also ends the sessions
	viewerToken, err = s.Login(ctx, "viewer", "viewerpw")
	require.NoError(t, err)
	require.NoError(t, s.ChangeUserPassword(ctx, "viewer", "reset"))
	_, err = s.AuthenticateSession(ctx, "viewer", viewerToken)
	assert.ErrorIs(t, err, user.ErrAccessDenied)
}

func TestUserService_PasswordTooLong(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)

	long := strings.Repeat("a", user.MaxPasswordLength+1)

	assert.ErrorIs(t, s.CreateUser(ctx, models.User{Username: "admin", Role: adminRole}, long), user.ErrPasswordTooLong)
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: adminRole}, "pw"))
	assert.ErrorIs(t, s.ChangeUserPassword(ctx, "admin", long), user.ErrPasswordTooLong)
	assert.ErrorIs(t, s.ChangePassword(ctx, "admin", "pw", long), user.ErrPasswordTooLong)
	assert.NoError(t, s.ValidateCredentials(ctx, "admin", "pw"))
}

func newAPIKey(t *testing.T, username string) string {
	t.Helper()

	key, err := manager.GenerateAPIKey(username)
	require.NoError(t, err)
	return key
}

func TestUserService_APIKey(t *testing.T) {
	ctx := context.Background()
	s, cfg := newTestService(t)

	_, err := s.AuthenticateByAPIKey(ctx, "anything")
	assert.ErrorIs(t, err, user.ErrAccessDenied, "no API key configured")

	// without users, the key authenticates as an admin
	key := newAPIKey(t, "admin")
	cfg.SetString(config.ApiKey, key)
	u, err := s.AuthenticateByAPIKey(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, adminRole, u.Role)
	_, err = s.AuthenticateByAPIKey(ctx, "wrong-key")
	assert.ErrorIs(t, err, user.ErrAccessDenied)

	// with users, the user of the key must exist and be an admin
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: adminRole}, "pw"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin2", Role: adminRole}, "pw"))
	u, err = s.AuthenticateByAPIKey(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, "admin", u.Username)
	assert.Equal(t, adminRole, u.Role)

	require.NoError(t, s.UpdateUser(ctx, "admin", models.User{Username: "admin", Role: modifyRole}))
	_, err = s.AuthenticateByAPIKey(ctx, key)
	assert.ErrorIs(t, err, user.ErrAccessDenied, "demoted user")

	require.NoError(t, s.UpdateUser(ctx, "admin", models.User{Username: "admin", Role: adminRole}))
	_, err = s.AuthenticateByAPIKey(ctx, key)
	require.NoError(t, err)

	require.NoError(t, s.DeleteUser(ctx, "admin"))
	_, err = s.AuthenticateByAPIKey(ctx, key)
	assert.ErrorIs(t, err, user.ErrAccessDenied, "deleted user")

	// a key without a user claim cannot be used once users exist
	cfg.SetString(config.ApiKey, "global-key")
	_, err = s.AuthenticateByAPIKey(ctx, "global-key")
	assert.ErrorIs(t, err, user.ErrAccessDenied)
}

func TestUserService_AutoLoginUser(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)

	u, _, err := s.AutoLoginUser(ctx)
	require.NoError(t, err)
	assert.Nil(t, u, "no users")

	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: adminRole}, "adminpw"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "editor", Role: modifyRole}, "editorpw"))

	u, _, err = s.AutoLoginUser(ctx)
	require.NoError(t, err)
	assert.Nil(t, u, "no user marked")

	// the token of the marked user authenticates its sessions, like the
	// token of a password login
	require.NoError(t, s.UpdateUser(ctx, "editor", models.User{Username: "editor", Role: modifyRole, AutoLogin: true}))
	u, token, err := s.AutoLoginUser(ctx)
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, "editor", u.Username)
	assert.Equal(t, modifyRole, u.Role)
	loginToken, err := s.Login(ctx, "editor", "editorpw")
	require.NoError(t, err)
	assert.Equal(t, loginToken, token)
	_, err = s.AuthenticateSession(ctx, "editor", token)
	assert.NoError(t, err)

	// a password change ends the sessions; the new token is returned
	require.NoError(t, s.ChangeUserPassword(ctx, "editor", "new"))
	_, err = s.AuthenticateSession(ctx, "editor", token)
	assert.ErrorIs(t, err, user.ErrAccessDenied)
	u, newToken, err := s.AutoLoginUser(ctx)
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.NotEqual(t, token, newToken)
	_, err = s.AuthenticateSession(ctx, "editor", newToken)
	assert.NoError(t, err)

	// a locked user is not logged in automatically
	require.NoError(t, s.UpdateUser(ctx, "editor", models.User{Username: "editor", AutoLogin: true}))
	u, _, err = s.AutoLoginUser(ctx)
	require.NoError(t, err)
	assert.Nil(t, u, "locked user")

	// nor is a removed user
	require.NoError(t, s.UpdateUser(ctx, "editor", models.User{Username: "editor", Role: modifyRole, AutoLogin: true}))
	require.NoError(t, s.DeleteUser(ctx, "editor"))
	u, _, err = s.AutoLoginUser(ctx)
	require.NoError(t, err)
	assert.Nil(t, u, "removed user")

	// nor is a user whose mark was cleared
	require.NoError(t, s.UpdateUser(ctx, "admin", models.User{Username: "admin", Role: adminRole, AutoLogin: true}))
	u, _, err = s.AutoLoginUser(ctx)
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, "admin", u.Username)
	require.NoError(t, s.UpdateUser(ctx, "admin", models.User{Username: "admin", Role: adminRole}))
	u, _, err = s.AutoLoginUser(ctx)
	require.NoError(t, err)
	assert.Nil(t, u, "mark cleared")
}

// newFirstAdminService returns a user service with the first admin, which has
// no password and auto login.
func newFirstAdminService(t *testing.T) *user.Service {
	t.Helper()

	s, cfg := newTestService(t)
	created, err := cfg.UserStore.CreateFirstAdmin(context.Background())
	require.NoError(t, err)
	require.True(t, created)

	return s
}

func TestUserService_FirstPassword(t *testing.T) {
	ctx := context.Background()
	s := newFirstAdminService(t)

	// a user without a password is never logged in with a password
	_, err := s.Login(ctx, "admin", "")
	assert.ErrorIs(t, err, user.ErrAccessDenied)
	_, err = s.Login(ctx, "admin", "anything")
	assert.ErrorIs(t, err, user.ErrAccessDenied)

	// and sets the first password without the current one
	require.NoError(t, s.ChangePassword(ctx, "admin", "", "first"))
	assert.NoError(t, s.ValidateCredentials(ctx, "admin", "first"))
	u, err := s.GetUser(ctx, "admin")
	require.NoError(t, err)
	assert.True(t, u.HasPassword)

	// once the user has a password, the current password is required
	assert.ErrorIs(t, s.ChangePassword(ctx, "admin", "", "second"), user.ErrCurrentPasswordIncorrect)
	assert.ErrorIs(t, s.ChangePassword(ctx, "admin", "wrong", "second"), user.ErrCurrentPasswordIncorrect)
	assert.NoError(t, s.ValidateCredentials(ctx, "admin", "first"))
	require.NoError(t, s.ChangePassword(ctx, "admin", "first", "second"))
	assert.NoError(t, s.ValidateCredentials(ctx, "admin", "second"))
}

func TestUserService_AutoLoginWithoutPassword(t *testing.T) {
	ctx := context.Background()
	s := newFirstAdminService(t)
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "editor", Role: modifyRole}, "pw"))

	// auto login cannot be turned off for the user without a password
	assert.ErrorIs(t, s.UpdateUser(ctx, "admin", models.User{Username: "admin", Role: adminRole}), user.ErrAutoLoginNoPassword)

	// nor given to another user, which would turn it off for that user
	assert.ErrorIs(t, s.UpdateUser(ctx, "editor", models.User{Username: "editor", Role: modifyRole, AutoLogin: true}), user.ErrAutoLoginUserNoPassword)
	assert.ErrorIs(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: readRole, AutoLogin: true}, "pw"), user.ErrAutoLoginUserNoPassword)

	// other changes are allowed
	require.NoError(t, s.UpdateUser(ctx, "editor", models.User{Username: "editor", Role: readRole}))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: readRole}, "pw"))

	u, _, err := s.AutoLoginUser(ctx)
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, "admin", u.Username)

	// once the user has a password, auto login can be moved and turned off
	require.NoError(t, s.ChangeUserPassword(ctx, "admin", "pw"))
	require.NoError(t, s.UpdateUser(ctx, "editor", models.User{Username: "editor", Role: readRole, AutoLogin: true}))
	u, _, err = s.AutoLoginUser(ctx)
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, "editor", u.Username)

	require.NoError(t, s.UpdateUser(ctx, "editor", models.User{Username: "editor", Role: readRole}))
	u, _, err = s.AutoLoginUser(ctx)
	require.NoError(t, err)
	assert.Nil(t, u)
}

// TestUserService_RoleValidation checks the role names and permissions that
// create and update reject.
func TestUserService_RoleValidation(t *testing.T) {
	ctx := context.Background()
	s, cfg := newTestService(t)

	custom, err := s.CreateRole(ctx, models.Role{Name: "  Tagger  ", Permissions: models.Permissions{models.PermissionTagsEdit}})
	require.NoError(t, err)
	assert.Equal(t, "Tagger", custom.Name, "trimmed name")

	tests := []struct {
		name        string
		roleName    string
		permissions models.Permissions
		want        error
	}{
		{"empty name", "", nil, user.ErrEmptyRoleName},
		{"name of spaces", "   ", nil, user.ErrEmptyRoleName},
		{"invalid permission", "Marker editor", models.Permissions{"NOPE"}, user.ErrInvalidPermission},
		{"duplicate permission", "Marker editor", models.Permissions{models.PermissionMarkersEdit, models.PermissionMarkersEdit}, user.ErrInvalidPermission},
		{"name of another role", "editor", nil, user.ErrRoleNameExists},
		{"name of another role with spaces", " VIEWER ", nil, user.ErrRoleNameExists},
		{"name of the Admin role", "admin", nil, user.ErrRoleNameExists},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.CreateRole(ctx, models.Role{Name: tt.roleName, Permissions: tt.permissions})
			assert.ErrorIs(t, err, tt.want, "create")

			err = s.UpdateRole(ctx, models.Role{ID: custom.ID, Name: tt.roleName, Permissions: tt.permissions})
			assert.ErrorIs(t, err, tt.want, "update")
		})
	}

	require.NoError(t, s.UpdateRole(ctx, models.Role{ID: custom.ID, Name: " Tag editor ", Permissions: models.Permissions{models.PermissionTagsEdit}}))
	got, err := cfg.UserStore.GetRole(ctx, custom.ID)
	require.NoError(t, err)
	assert.Equal(t, "Tag editor", got.Name, "trimmed name")

	assert.ErrorIs(t, s.DeleteRole(ctx, "nope"), user.ErrRoleNotExist)
	require.NoError(t, s.DeleteRole(ctx, custom.ID))

	roles, err := s.AllRoles(ctx)
	require.NoError(t, err)
	assert.Len(t, roles, 3, "Admin and the default roles")
}
