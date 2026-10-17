package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestUserStore returns a user store backed by a config file in a
// temporary directory, with the provided initial file contents.
func newTestUserStore(t *testing.T, contents string) (*UserStore, string) {
	t.Helper()

	fn := filepath.Join(t.TempDir(), "config.yml")
	require.NoError(t, os.WriteFile(fn, []byte(contents), 0600))

	return loadTestUserStore(t, fn), fn
}

// loadTestUserStore loads a new config instance from the file.
func loadTestUserStore(t *testing.T, fn string) *UserStore {
	t.Helper()

	cfg := newConfig()
	require.NoError(t, cfg.load(fn))

	return cfg.UserStore
}

func mustHash(t *testing.T, password string) string {
	t.Helper()

	hash, err := hashPassword(password)
	require.NoError(t, err)
	return hash
}

func usernames(t *testing.T, s *UserStore) []string {
	t.Helper()

	users, err := s.AllUsers(context.Background())
	require.NoError(t, err)

	var ret []string
	for _, u := range users {
		ret = append(ret, u.Username)
	}
	return ret
}

func TestUserStore_NoUsers(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestUserStore(t, "")

	assert.False(t, s.LoginRequired(ctx))
	assert.Empty(t, usernames(t, s))
	assert.False(t, s.ValidateCredentials(ctx, "", ""))

	u, err := s.GetUser(ctx, "admin")
	assert.NoError(t, err)
	assert.Nil(t, u)
}

func TestUserStore_LegacyUser(t *testing.T) {
	ctx := context.Background()
	hash := mustHash(t, "secret")
	s, _ := newTestUserStore(t, "username: admin\npassword: "+hash+"\n")

	assert.True(t, s.LoginRequired(ctx))
	assert.Equal(t, []string{"admin"}, usernames(t, s))

	u, err := s.GetUser(ctx, "admin")
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, models.RoleIDAdmin, u.Role, "legacy user must be an admin")

	assert.True(t, s.ValidateCredentials(ctx, "admin", "secret"))
	assert.False(t, s.ValidateCredentials(ctx, "admin", "wrong"))
	assert.False(t, s.ValidateCredentials(ctx, "other", "secret"))
}

func TestUserStore_LegacyUserIncomplete(t *testing.T) {
	ctx := context.Background()

	// a username without a password does not define a user
	s, _ := newTestUserStore(t, "username: admin\n")
	assert.False(t, s.LoginRequired(ctx))
	assert.Empty(t, usernames(t, s))
}

func TestUserStore_CreateUpdateDelete(t *testing.T) {
	ctx := context.Background()
	s, fn := newTestUserStore(t, "")

	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: models.RoleIDAdmin}, "adminpw"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "editor", Role: models.RoleIDEditor}, "editorpw"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: models.RoleIDViewer}, "viewerpw"))

	assert.Error(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: models.RoleIDViewer}, "x"), "duplicate username")

	assert.True(t, s.LoginRequired(ctx))
	assert.Equal(t, []string{"admin", "editor", "viewer"}, usernames(t, s))
	assert.True(t, s.ValidateCredentials(ctx, "editor", "editorpw"))

	// only password hashes are written to the config file
	data, err := os.ReadFile(fn)
	require.NoError(t, err)
	for _, pw := range []string{"adminpw", "editorpw", "viewerpw"} {
		assert.NotContains(t, string(data), pw)
	}

	// users persist across reloads
	reloaded := loadTestUserStore(t, fn)
	assert.Equal(t, []string{"admin", "editor", "viewer"}, usernames(t, reloaded))
	u, err := reloaded.GetUser(ctx, "editor")
	require.NoError(t, err)
	assert.Equal(t, models.RoleIDEditor, u.Role)
	assert.True(t, reloaded.ValidateCredentials(ctx, "viewer", "viewerpw"))

	// update role and name - password is unchanged
	require.NoError(t, s.ReplaceUser(ctx, "viewer", models.User{Username: "viewer2", Role: models.RoleIDEditor}))
	u, err = s.GetUser(ctx, "viewer2")
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, models.RoleIDEditor, u.Role)
	assert.True(t, s.ValidateCredentials(ctx, "viewer2", "viewerpw"))
	u, err = s.GetUser(ctx, "viewer")
	require.NoError(t, err)
	assert.Nil(t, u)

	assert.Error(t, s.ReplaceUser(ctx, "viewer2", models.User{Username: "editor", Role: models.RoleIDViewer}), "rename to existing username")
	assert.Error(t, s.ReplaceUser(ctx, "missing", models.User{Username: "missing", Role: models.RoleIDViewer}))

	// change password
	require.NoError(t, s.ChangeUserPassword(ctx, "editor", "newpw"))
	assert.False(t, s.ValidateCredentials(ctx, "editor", "editorpw"))
	assert.True(t, s.ValidateCredentials(ctx, "editor", "newpw"))
	assert.Error(t, s.ChangeUserPassword(ctx, "missing", "x"))

	// delete
	require.NoError(t, s.DeleteUser(ctx, "viewer2"))
	assert.Equal(t, []string{"admin", "editor"}, usernames(t, s))
	assert.Error(t, s.DeleteUser(ctx, "viewer2"))

	reloaded = loadTestUserStore(t, fn)
	assert.Equal(t, []string{"admin", "editor"}, usernames(t, reloaded))
	assert.True(t, reloaded.ValidateCredentials(ctx, "editor", "newpw"))

	// the last admin cannot be removed
	require.NoError(t, s.DeleteUser(ctx, "editor"))
	assert.ErrorIs(t, s.DeleteUser(ctx, "admin"), user.ErrDeleteLastAdminUser)
	assert.True(t, loadTestUserStore(t, fn).LoginRequired(ctx))
}

func TestUserStore_Invariants(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestUserStore(t, "")

	adminRole := models.RoleIDAdmin
	readRole := models.RoleIDViewer

	assert.ErrorIs(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: readRole}, "pw"), user.ErrFirstUserNotAdmin)
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: adminRole}, "pw"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: readRole}, "pw"))

	tests := []struct {
		name string
		fn   func() error
		want error
	}{
		{"duplicate username", func() error { return s.CreateUser(ctx, models.User{Username: "admin", Role: adminRole}, "pw") }, user.ErrUserAlreadyExists},
		{"demote last admin", func() error { return s.ReplaceUser(ctx, "admin", models.User{Username: "admin", Role: readRole}) }, user.ErrRemoveLastAdminRole},
		{"delete last admin", func() error { return s.DeleteUser(ctx, "admin") }, user.ErrDeleteLastAdminUser},
		{"replace missing user", func() error { return s.ReplaceUser(ctx, "missing", models.User{Username: "missing", Role: readRole}) }, user.ErrUserNotExist},
		{"delete missing user", func() error { return s.DeleteUser(ctx, "missing") }, user.ErrUserNotExist},
		{"change password of missing user", func() error { return s.ChangeUserPassword(ctx, "missing", "pw") }, user.ErrUserNotExist},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, tt.fn(), tt.want)
		})
	}
}

// TestUserStore_LastAdminConcurrent removes one admin and demotes the other
// concurrently: one admin must always remain.
func TestUserStore_LastAdminConcurrent(t *testing.T) {
	ctx := context.Background()

	for i := 0; i < 20; i++ {
		s, fn := newTestUserStore(t, "")
		require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin1", Role: models.RoleIDAdmin}, "pw"))
		require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin2", Role: models.RoleIDAdmin}, "pw"))

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = s.DeleteUser(ctx, "admin1")
		}()
		go func() {
			defer wg.Done()
			_ = s.ReplaceUser(ctx, "admin2", models.User{Username: "admin2", Role: models.RoleIDViewer})
		}()
		wg.Wait()

		users, err := loadTestUserStore(t, fn).AllUsers(ctx)
		require.NoError(t, err)
		admins := 0
		for _, u := range users {
			if u.IsAdmin() {
				admins++
			}
		}
		assert.Equal(t, 1, admins)
	}
}

func TestUserStore_PasswordTooLong(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestUserStore(t, "")

	long := strings.Repeat("a", user.MaxPasswordLength+1)
	maxLength := strings.Repeat("a", user.MaxPasswordLength)
	adminRole := models.RoleIDAdmin

	assert.ErrorIs(t, s.CreateUser(ctx, models.User{Username: "admin", Role: adminRole}, long), user.ErrPasswordTooLong)
	assert.False(t, s.LoginRequired(ctx), "user must not be created")

	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: adminRole}, maxLength))
	assert.True(t, s.ValidateCredentials(ctx, "admin", maxLength))

	assert.ErrorIs(t, s.ChangeUserPassword(ctx, "admin", long), user.ErrPasswordTooLong)
	assert.True(t, s.ValidateCredentials(ctx, "admin", maxLength), "password must be unchanged")
}

func TestUserStore_SessionToken(t *testing.T) {
	ctx := context.Background()
	s, fn := newTestUserStore(t, "username: admin\npassword: "+mustHash(t, "secret")+"\n")
	readRole := models.RoleIDViewer

	// the legacy user has a token derived from the password hash
	legacyToken, err := s.GetSessionToken(ctx, "admin")
	require.NoError(t, err)
	require.NotEmpty(t, legacyToken)

	// moving the legacy user into the users list keeps its sessions
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: readRole}, "pw"))
	token, err := s.GetSessionToken(ctx, "admin")
	require.NoError(t, err)
	assert.Equal(t, legacyToken, token)

	viewerToken, err := s.GetSessionToken(ctx, "viewer")
	require.NoError(t, err)
	assert.NotEmpty(t, viewerToken)
	assert.NotEqual(t, legacyToken, viewerToken)

	// tokens persist across reloads
	token, err = loadTestUserStore(t, fn).GetSessionToken(ctx, "viewer")
	require.NoError(t, err)
	assert.Equal(t, viewerToken, token)

	// changing the password replaces the token
	require.NoError(t, s.ChangeUserPassword(ctx, "viewer", "new"))
	token, err = s.GetSessionToken(ctx, "viewer")
	require.NoError(t, err)
	assert.NotEqual(t, viewerToken, token)
	viewerToken = token

	require.NoError(t, s.ChangeUserPassword(ctx, "admin", "new"))
	token, err = s.GetSessionToken(ctx, "admin")
	require.NoError(t, err)
	assert.NotEqual(t, legacyToken, token)

	// a user created again with the same name has a different token
	require.NoError(t, s.DeleteUser(ctx, "viewer"))
	_, err = s.GetSessionToken(ctx, "viewer")
	assert.ErrorIs(t, err, user.ErrUserNotExist)
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: readRole}, "new"))
	token, err = s.GetSessionToken(ctx, "viewer")
	require.NoError(t, err)
	assert.NotEqual(t, viewerToken, token)
}

func TestUserStore_SetLegacyCredentials(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestUserStore(t, "")

	username := "admin"
	password := "secret"
	long := strings.Repeat("a", user.MaxPasswordLength+1)

	// without a users list, the legacy credentials can be set
	assert.ErrorIs(t, s.SetLegacyCredentials(&username, &long), user.ErrPasswordTooLong)
	assert.False(t, s.LoginRequired(ctx))

	require.NoError(t, s.SetLegacyCredentials(&username, &password))
	assert.True(t, s.LoginRequired(ctx))
	assert.True(t, s.ValidateCredentials(ctx, "admin", "secret"))

	// passing the stored hash leaves the password unchanged
	hash := s.GetPasswordHash()
	require.NoError(t, s.SetLegacyCredentials(&username, &hash))
	assert.True(t, s.ValidateCredentials(ctx, "admin", "secret"))

	// once users are listed, the legacy credentials cannot be changed
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: models.RoleIDViewer}, "pw"))
	other := "other"
	empty := ""
	assert.ErrorIs(t, s.SetLegacyCredentials(&other, &password), ErrUsersListExists)
	assert.ErrorIs(t, s.SetLegacyCredentials(nil, &password), ErrUsersListExists)
	assert.ErrorIs(t, s.SetLegacyCredentials(&other, nil), ErrUsersListExists)

	// unchanged (empty) values are accepted
	require.NoError(t, s.SetLegacyCredentials(&empty, &empty))
	require.NoError(t, s.SetLegacyCredentials(nil, nil))
	assert.Equal(t, []string{"admin", "viewer"}, usernames(t, s))
	assert.Equal(t, "", s.GetUsername())
}

func TestUserStore_LegacyUserMigration(t *testing.T) {
	ctx := context.Background()
	hash := mustHash(t, "secret")
	s, fn := newTestUserStore(t, "username: admin\npassword: "+hash+"\napi_key: key\n")

	// modifying users moves the legacy user into the users list
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: models.RoleIDViewer}, "viewerpw"))

	reloaded := loadTestUserStore(t, fn)
	assert.Equal(t, []string{"admin", "viewer"}, usernames(t, reloaded))
	assert.Equal(t, "", reloaded.GetUsername(), "legacy username must be removed")
	assert.Equal(t, "", reloaded.GetPasswordHash(), "legacy password must be removed")
	assert.Equal(t, "key", reloaded.GetAPIKey(), "other settings must be kept")

	// legacy user keeps its password and the admin role
	assert.True(t, reloaded.ValidateCredentials(ctx, "admin", "secret"))
	u, err := reloaded.GetUser(ctx, "admin")
	require.NoError(t, err)
	assert.Equal(t, models.RoleIDAdmin, u.Role)

	data, err := os.ReadFile(fn)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), hash), "legacy hash must be stored once")
}

func TestUserStore_LegacyPasswordChange(t *testing.T) {
	ctx := context.Background()
	s, fn := newTestUserStore(t, "username: admin\npassword: "+mustHash(t, "secret")+"\n")

	require.NoError(t, s.ChangeUserPassword(ctx, "admin", "changed"))

	reloaded := loadTestUserStore(t, fn)
	assert.Equal(t, []string{"admin"}, usernames(t, reloaded))
	assert.False(t, reloaded.ValidateCredentials(ctx, "admin", "secret"))
	assert.True(t, reloaded.ValidateCredentials(ctx, "admin", "changed"))
}

func TestUserStore_ListedUserOverridesLegacyUser(t *testing.T) {
	ctx := context.Background()
	contents := "username: admin\npassword: " + mustHash(t, "legacy") + "\n" +
		"users:\n" +
		"  - username: admin\n" +
		"    passwordhash: " + mustHash(t, "listed") + "\n" +
		"    roles: [ADMIN]\n" +
		"  - username: viewer\n" +
		"    passwordhash: " + mustHash(t, "viewerpw") + "\n" +
		"    roles: [READ]\n"
	s, _ := newTestUserStore(t, contents)

	assert.Equal(t, []string{"admin", "viewer"}, usernames(t, s))
	assert.True(t, s.ValidateCredentials(ctx, "admin", "listed"))
	assert.False(t, s.ValidateCredentials(ctx, "admin", "legacy"))

	u, err := s.GetUser(ctx, "viewer")
	require.NoError(t, err)
	assert.Equal(t, models.RoleIDViewer, u.Role)
}

// autoLoginUsers returns the names of the users marked for auto login.
func autoLoginUsers(t *testing.T, s *UserStore) []string {
	t.Helper()

	users, err := s.AllUsers(context.Background())
	require.NoError(t, err)

	var ret []string
	for _, u := range users {
		if u.AutoLogin {
			ret = append(ret, u.Username)
		}
	}
	return ret
}

func TestUserStore_AutoLogin(t *testing.T) {
	ctx := context.Background()
	s, fn := newTestUserStore(t, "username: admin\npassword: "+mustHash(t, "secret")+"\n")

	adminRole := models.RoleIDAdmin
	modifyRole := models.RoleIDEditor
	readRole := models.RoleIDViewer

	assert.Empty(t, autoLoginUsers(t, s))

	// the legacy user can be marked - it is moved into the users list
	require.NoError(t, s.ReplaceUser(ctx, "admin", models.User{Username: "admin", Role: adminRole, AutoLogin: true}))
	assert.Equal(t, []string{"admin"}, autoLoginUsers(t, s))

	// creating a marked user clears the mark of every other user
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "editor", Role: modifyRole, AutoLogin: true}, "pw"))
	assert.Equal(t, []string{"editor"}, autoLoginUsers(t, s))

	// creating an unmarked user leaves the mark unchanged
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: readRole}, "pw"))
	assert.Equal(t, []string{"editor"}, autoLoginUsers(t, s))

	// marking another user clears the mark of every other user
	require.NoError(t, s.ReplaceUser(ctx, "viewer", models.User{Username: "viewer", Role: readRole, AutoLogin: true}))
	assert.Equal(t, []string{"viewer"}, autoLoginUsers(t, s))

	// the mark persists across reloads, and moves with a renamed user
	assert.Equal(t, []string{"viewer"}, autoLoginUsers(t, loadTestUserStore(t, fn)))
	require.NoError(t, s.ReplaceUser(ctx, "viewer", models.User{Username: "viewer2", Role: readRole, AutoLogin: true}))
	assert.Equal(t, []string{"viewer2"}, autoLoginUsers(t, loadTestUserStore(t, fn)))

	// the mark is only stored for the marked user
	data, err := os.ReadFile(fn)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), "autologin"))

	// changing the password keeps the mark
	require.NoError(t, s.ChangeUserPassword(ctx, "viewer2", "new"))
	assert.Equal(t, []string{"viewer2"}, autoLoginUsers(t, s))

	// clearing the mark
	require.NoError(t, s.ReplaceUser(ctx, "viewer2", models.User{Username: "viewer2", Role: readRole}))
	assert.Empty(t, autoLoginUsers(t, s))
	data, err = os.ReadFile(fn)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "autologin", "mark is omitted when false")

	// removing the marked user removes the mark
	require.NoError(t, s.ReplaceUser(ctx, "editor", models.User{Username: "editor", Role: modifyRole, AutoLogin: true}))
	require.NoError(t, s.DeleteUser(ctx, "editor"))
	assert.Empty(t, autoLoginUsers(t, s))
	assert.Empty(t, autoLoginUsers(t, loadTestUserStore(t, fn)))
}

// TestUserStore_AutoLoginConcurrent marks two users concurrently: at most one
// user must be marked.
func TestUserStore_AutoLoginConcurrent(t *testing.T) {
	ctx := context.Background()
	readRole := models.RoleIDViewer

	for i := 0; i < 20; i++ {
		s, fn := newTestUserStore(t, "")
		require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: models.RoleIDAdmin}, "pw"))
		require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer1", Role: readRole}, "pw"))

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = s.ReplaceUser(ctx, "viewer1", models.User{Username: "viewer1", Role: readRole, AutoLogin: true})
		}()
		go func() {
			defer wg.Done()
			_ = s.CreateUser(ctx, models.User{Username: "viewer2", Role: readRole, AutoLogin: true}, "pw")
		}()
		wg.Wait()

		assert.Len(t, autoLoginUsers(t, loadTestUserStore(t, fn)), 1)
	}
}

func TestUserStore_CreateFirstAdmin(t *testing.T) {
	ctx := context.Background()
	s, fn := newTestUserStore(t, "")

	created, err := s.CreateFirstAdmin(ctx)
	require.NoError(t, err)
	assert.True(t, created)

	// the admin is stored, has no password and is logged in automatically
	reloaded := loadTestUserStore(t, fn)
	assert.True(t, reloaded.LoginRequired(ctx))
	assert.Equal(t, []string{FirstAdminUsername}, usernames(t, reloaded))
	assert.Equal(t, []string{FirstAdminUsername}, autoLoginUsers(t, reloaded))

	u, err := reloaded.GetUser(ctx, FirstAdminUsername)
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, models.RoleIDAdmin, u.Role)
	assert.False(t, u.HasPassword)
	assert.False(t, reloaded.ValidateCredentials(ctx, FirstAdminUsername, ""))

	// it is created once
	created, err = reloaded.CreateFirstAdmin(ctx)
	require.NoError(t, err)
	assert.False(t, created)
	assert.Equal(t, []string{FirstAdminUsername}, usernames(t, loadTestUserStore(t, fn)))
}

func TestUserStore_CreateFirstAdminLegacyUser(t *testing.T) {
	ctx := context.Background()
	hash := mustHash(t, "secret")
	s, fn := newTestUserStore(t, "username: legacy\npassword: "+hash+"\n")

	// the legacy user is the first admin, with its password
	created, err := s.CreateFirstAdmin(ctx)
	require.NoError(t, err)
	assert.False(t, created)

	reloaded := loadTestUserStore(t, fn)
	assert.Equal(t, []string{"legacy"}, usernames(t, reloaded))
	assert.Empty(t, autoLoginUsers(t, reloaded))
	assert.Equal(t, hash, reloaded.GetPasswordHash())

	u, err := reloaded.GetUser(ctx, "legacy")
	require.NoError(t, err)
	require.NotNil(t, u)
	assert.Equal(t, models.RoleIDAdmin, u.Role)
	assert.True(t, u.HasPassword)
	assert.True(t, reloaded.ValidateCredentials(ctx, "legacy", "secret"))
}

func TestUserStore_CreateFirstAdminExistingUsers(t *testing.T) {
	ctx := context.Background()
	s, fn := newTestUserStore(t, "")
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "root", Role: models.RoleIDAdmin}, "pw"))

	before, err := os.ReadFile(fn)
	require.NoError(t, err)

	created, err := s.CreateFirstAdmin(ctx)
	require.NoError(t, err)
	assert.False(t, created)

	after, err := os.ReadFile(fn)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the config file must not change")
	assert.Equal(t, []string{"root"}, usernames(t, loadTestUserStore(t, fn)))
}
