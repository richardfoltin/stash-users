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
	adminRole  = models.Roles{models.RoleEnumAdmin}
	modifyRole = models.Roles{models.RoleEnumModify}
	readRole   = models.Roles{models.RoleEnumRead}
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
	assert.ErrorIs(t, s.CreateUser(ctx, models.User{Username: "viewer", Roles: readRole}, "pw"), user.ErrFirstUserNotAdmin)
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Roles: adminRole}, "pw"))
	assert.True(t, s.LoginRequired(ctx))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Roles: readRole}, "pw"))
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
		{"empty username", models.User{Username: "", Roles: adminRole}, "pw", user.ErrEmptyUsername},
		{"username with whitespace", models.User{Username: " admin", Roles: adminRole}, "pw", user.ErrUsernameHasWhitespace},
		{"empty password", models.User{Username: "admin", Roles: adminRole}, "", user.ErrEmptyPassword},
		{"invalid role", models.User{Username: "admin", Roles: models.Roles{"SUPERUSER"}}, "pw", user.ErrInvalidRole},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, s.CreateUser(ctx, tt.user, tt.password), tt.want)
		})
	}

	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Roles: adminRole}, "pw"))
	assert.ErrorIs(t, s.CreateUser(ctx, models.User{Username: "admin", Roles: adminRole}, "pw"), user.ErrUserAlreadyExists)
}

func TestUserService_LastAdminProtection(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)

	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Roles: adminRole}, "pw"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "editor", Roles: modifyRole}, "pw"))

	// the last admin cannot be demoted or removed - even if it is the only user
	assert.ErrorIs(t, s.UpdateUser(ctx, "admin", models.User{Username: "admin", Roles: modifyRole}), user.ErrRemoveLastAdminRole)
	assert.ErrorIs(t, s.UpdateUser(ctx, "admin", models.User{Username: "admin", Roles: models.Roles{}}), user.ErrRemoveLastAdminRole)
	assert.ErrorIs(t, s.DeleteUser(ctx, "admin"), user.ErrDeleteLastAdminUser)

	// renaming the last admin is allowed
	require.NoError(t, s.UpdateUser(ctx, "admin", models.User{Username: "root", Roles: adminRole}))

	// non-admins can be removed
	require.NoError(t, s.DeleteUser(ctx, "editor"))
	assert.ErrorIs(t, s.DeleteUser(ctx, "root"), user.ErrDeleteLastAdminUser)

	// with a second admin, either admin can be demoted or removed
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin2", Roles: adminRole}, "pw"))
	require.NoError(t, s.UpdateUser(ctx, "root", models.User{Username: "root", Roles: readRole}))
	assert.ErrorIs(t, s.DeleteUser(ctx, "admin2"), user.ErrDeleteLastAdminUser)
	require.NoError(t, s.UpdateUser(ctx, "root", models.User{Username: "root", Roles: adminRole}))
	require.NoError(t, s.DeleteUser(ctx, "admin2"))

	users, err := s.AllUsers(ctx)
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, "root", users[0].Username)
	assert.Equal(t, adminRole, users[0].Roles)

	assert.ErrorIs(t, s.UpdateUser(ctx, "missing", models.User{Username: "missing", Roles: readRole}), user.ErrUserNotExist)
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
	assert.Equal(t, adminRole, u.Roles)

	// the legacy user is the last admin
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Roles: readRole}, "pw"))
	assert.ErrorIs(t, s.DeleteUser(ctx, "legacy"), user.ErrDeleteLastAdminUser)
	assert.ErrorIs(t, s.UpdateUser(ctx, "legacy", models.User{Username: "legacy", Roles: readRole}), user.ErrRemoveLastAdminRole)

	// and can still log in after being moved into the users list, keeping
	// existing sessions
	assert.NoError(t, s.ValidateCredentials(ctx, "legacy", "secret"))
	_, err = s.AuthenticateSession(ctx, "legacy", token)
	assert.NoError(t, err)
}

func TestUserService_Authentication(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestService(t)

	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Roles: adminRole}, "adminpw"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Roles: readRole}, "viewerpw"))

	assert.NoError(t, s.ValidateCredentials(ctx, "viewer", "viewerpw"))
	assert.ErrorIs(t, s.ValidateCredentials(ctx, "viewer", "adminpw"), user.ErrAccessDenied)
	assert.ErrorIs(t, s.ValidateCredentials(ctx, "missing", "pw"), user.ErrAccessDenied)

	_, err := s.Login(ctx, "viewer", "adminpw")
	assert.ErrorIs(t, err, user.ErrAccessDenied)
	viewerToken, err := s.Login(ctx, "viewer", "viewerpw")
	require.NoError(t, err)

	u, err := s.AuthenticateSession(ctx, "viewer", viewerToken)
	require.NoError(t, err)
	assert.Equal(t, readRole, u.Roles)
	_, err = s.AuthenticateSession(ctx, "viewer", "")
	assert.ErrorIs(t, err, user.ErrAccessDenied, "session without token")
	_, err = s.AuthenticateSession(ctx, "viewer", "wrong")
	assert.ErrorIs(t, err, user.ErrAccessDenied)

	// a user without roles is locked
	require.NoError(t, s.UpdateUser(ctx, "viewer", models.User{Username: "viewer", Roles: models.Roles{}}))
	assert.ErrorIs(t, s.ValidateCredentials(ctx, "viewer", "viewerpw"), user.ErrAccessDenied)
	_, err = s.AuthenticateSession(ctx, "viewer", viewerToken)
	assert.ErrorIs(t, err, user.ErrAccessDenied)

	// deleted users cannot authenticate
	require.NoError(t, s.DeleteUser(ctx, "viewer"))
	_, err = s.AuthenticateSession(ctx, "viewer", viewerToken)
	assert.ErrorIs(t, err, user.ErrAccessDenied)

	// a user created again with the same name does not get the sessions of
	// the deleted user
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Roles: readRole}, "viewerpw"))
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

	assert.ErrorIs(t, s.CreateUser(ctx, models.User{Username: "admin", Roles: adminRole}, long), user.ErrPasswordTooLong)
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Roles: adminRole}, "pw"))
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
	assert.Equal(t, adminRole, u.Roles)
	_, err = s.AuthenticateByAPIKey(ctx, "wrong-key")
	assert.ErrorIs(t, err, user.ErrAccessDenied)

	// with users, the user of the key must exist and be an admin
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Roles: adminRole}, "pw"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin2", Roles: adminRole}, "pw"))
	u, err = s.AuthenticateByAPIKey(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, "admin", u.Username)
	assert.Equal(t, adminRole, u.Roles)

	require.NoError(t, s.UpdateUser(ctx, "admin", models.User{Username: "admin", Roles: modifyRole}))
	_, err = s.AuthenticateByAPIKey(ctx, key)
	assert.ErrorIs(t, err, user.ErrAccessDenied, "demoted user")

	require.NoError(t, s.UpdateUser(ctx, "admin", models.User{Username: "admin", Roles: adminRole}))
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
