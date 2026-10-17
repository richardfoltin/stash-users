package config

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/user"
	"golang.org/x/crypto/bcrypt"
)

const (
	Username = "username"
	Password = "password"
	Users    = "users"
)

// FirstAdminUsername is the name of the user created when no users are
// configured.
const FirstAdminUsername = "admin"

var ErrUsersListExists = errors.New("the username and password are managed in the users list: use the user management instead")

// StoredUser is a user as stored in the config file, with its password hash.
type StoredUser struct {
	Username     string            `json:"username" koanf:"username"`
	PasswordHash string            `json:"passwordhash" koanf:"passwordhash"`
	Roles        []models.RoleEnum `json:"roles" koanf:"roles"`
	// SessionToken is replaced when the password changes, ending the sessions
	SessionToken string `json:"sessiontoken" koanf:"sessiontoken"`
	AutoLogin    bool   `json:"autologin" koanf:"autologin"`
}

func (u StoredUser) toMap() map[string]interface{} {
	roles := make([]string, len(u.Roles))
	for i, r := range u.Roles {
		roles[i] = r.String()
	}

	ret := map[string]interface{}{
		"username":     u.Username,
		"passwordhash": u.PasswordHash,
		"roles":        roles,
	}

	if u.SessionToken != "" {
		ret["sessiontoken"] = u.SessionToken
	}

	if u.AutoLogin {
		ret["autologin"] = true
	}

	return ret
}

func (u StoredUser) toUser() *models.User {
	return &models.User{
		Username:    u.Username,
		Roles:       slices.Clone(u.Roles),
		AutoLogin:   u.AutoLogin,
		HasPassword: u.hasPassword(),
	}
}

func (u StoredUser) hasPassword() bool {
	return u.PasswordHash != ""
}

func (u StoredUser) isAdmin() bool {
	return models.Roles(u.Roles).HasRole(models.RoleEnumAdmin)
}

// sessionToken returns the token that the sessions of the user must contain.
// Users without a stored token, such as the legacy user, use a token derived
// from the password hash.
func (u StoredUser) sessionToken() string {
	if u.SessionToken != "" {
		return u.SessionToken
	}

	sum := sha256.Sum256([]byte("stash-session:" + u.PasswordHash))
	return hex.EncodeToString(sum[:])
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating session token: %w", err)
	}

	return hex.EncodeToString(b), nil
}

// UserStore stores the users in the users list of the config file. The legacy
// username and password are an admin user, moved into the list on the first
// change.
type UserStore struct {
	*Config
}

func (i *Config) GetUsername() string {
	return i.getString(Username)
}

func (i *Config) GetPasswordHash() string {
	return i.getString(Password)
}

func hashPassword(password string) (string, error) {
	if err := user.ValidatePassword(password); err != nil {
		return "", err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}

	return string(hash), nil
}

// SetLegacyCredentials sets the legacy username and password while there is no
// users list. A nil value or the stored hash is not changed, and an empty
// password clears it. The config file is not written.
func (s *UserStore) SetLegacyCredentials(username *string, password *string) error {
	s.Lock()
	defer s.Unlock()

	currentUsername := s.forKey(Username).String(Username)
	currentHash := s.forKey(Password).String(Password)

	usernameChanged := username != nil && *username != currentUsername
	passwordChanged := password != nil && *password != currentHash

	if !usernameChanged && !passwordChanged {
		return nil
	}

	listed, err := s.listedUsersNoLock()
	if err != nil {
		return err
	}

	if len(listed) > 0 {
		return ErrUsersListExists
	}

	var newHash string
	if passwordChanged && *password != "" {
		newHash, err = hashPassword(*password)
		if err != nil {
			return err
		}
	}

	if usernameChanged {
		s.set(Username, *username)
	}

	if passwordChanged {
		s.set(Password, newHash)
	}

	return nil
}

// legacyUserNoLock returns the legacy user, or nil. Assumes read lock held.
func (s *UserStore) legacyUserNoLock() *StoredUser {
	un := s.forKey(Username).String(Username)
	pwHash := s.forKey(Password).String(Password)

	if un == "" || pwHash == "" {
		return nil
	}

	return &StoredUser{
		Username:     un,
		PasswordHash: pwHash,
		Roles:        []models.RoleEnum{models.RoleEnumAdmin},
	}
}

// listedUsersNoLock assumes read lock held.
func (s *UserStore) listedUsersNoLock() ([]StoredUser, error) {
	var ret []StoredUser
	if err := s.forKey(Users).Unmarshal(Users, &ret); err != nil {
		return nil, fmt.Errorf("reading users from config: %w", err)
	}

	return ret, nil
}

// allUsersNoLock returns all users, including the legacy user. Assumes read
// lock held.
func (s *UserStore) allUsersNoLock() ([]StoredUser, error) {
	ret, err := s.listedUsersNoLock()
	if err != nil {
		return nil, err
	}

	legacy := s.legacyUserNoLock()
	if legacy != nil && s.findUser(ret, legacy.Username) == -1 {
		// legacy user is listed first
		ret = append([]StoredUser{*legacy}, ret...)
	}

	return ret, nil
}

func (s *UserStore) findUser(users []StoredUser, username string) int {
	return slices.IndexFunc(users, func(u StoredUser) bool {
		return u.Username == username
	})
}

func clearOtherAutoLogin(users []StoredUser, idx int) {
	for i := range users {
		if i != idx {
			users[i].AutoLogin = false
		}
	}
}

// otherAutoLoginHasNoPassword returns true if a user other than the user at
// idx is marked for auto login and has no password.
func otherAutoLoginHasNoPassword(users []StoredUser, idx int) bool {
	for i, u := range users {
		if i != idx && u.AutoLogin && !u.hasPassword() {
			return true
		}
	}

	return false
}

func hasOtherAdmin(users []StoredUser, username string) bool {
	return slices.ContainsFunc(users, func(u StoredUser) bool {
		return u.Username != username && u.isAdmin()
	})
}

func (s *UserStore) getStoredUser(username string) (*StoredUser, error) {
	s.RLock()
	defer s.RUnlock()

	users, err := s.allUsersNoLock()
	if err != nil {
		return nil, err
	}

	idx := s.findUser(users, username)
	if idx == -1 {
		return nil, nil
	}

	return &users[idx], nil
}

// GetUser returns the user with the provided username, or nil if not found.
func (s *UserStore) GetUser(ctx context.Context, username string) (*models.User, error) {
	u, err := s.getStoredUser(username)
	if err != nil || u == nil {
		return nil, err
	}

	return u.toUser(), nil
}

// AllUsers returns all users, including the legacy user.
func (s *UserStore) AllUsers(ctx context.Context) ([]*models.User, error) {
	s.RLock()
	defer s.RUnlock()

	users, err := s.allUsersNoLock()
	if err != nil {
		return nil, err
	}

	ret := make([]*models.User, len(users))
	for i, u := range users {
		ret[i] = u.toUser()
	}

	return ret, nil
}

// LoginRequired returns true if any users are configured.
// It is called for every request, so it does not read the users list.
func (s *UserStore) LoginRequired(ctx context.Context) bool {
	s.RLock()
	defer s.RUnlock()

	if s.legacyUserNoLock() != nil {
		return true
	}

	switch listed := s.forKey(Users).Get(Users).(type) {
	case nil:
		return false
	case []interface{}:
		return len(listed) > 0
	case []map[string]interface{}:
		return len(listed) > 0
	default:
		// fail closed - an unexpected value must not disable authentication
		return true
	}
}

func (s *UserStore) ValidateCredentials(ctx context.Context, username string, password string) bool {
	u, err := s.getStoredUser(username)
	if err != nil || u == nil {
		return false
	}

	// a user without a password can only be logged in automatically
	if !u.hasPassword() {
		return false
	}

	err = bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))

	return err == nil
}

func (s *UserStore) GetSessionToken(ctx context.Context, username string) (string, error) {
	u, err := s.getStoredUser(username)
	if err != nil {
		return "", err
	}

	if u == nil {
		return "", user.ErrUserNotExist
	}

	return u.sessionToken(), nil
}

// modifyUsers moves the legacy user into the users list, applies fn to the
// list and writes the config file. The write lock is held throughout, so fn
// can check invariants atomically.
func (s *UserStore) modifyUsers(fn func(users []StoredUser) ([]StoredUser, error)) error {
	s.Lock()
	defer s.Unlock()

	users, err := s.allUsersNoLock()
	if err != nil {
		return err
	}

	users, err = fn(users)
	if err != nil {
		return err
	}

	// the legacy user (if any) is moved into the users list
	s.main.Delete(Username)
	s.main.Delete(Password)

	stored := make([]map[string]interface{}, len(users))
	for i, u := range users {
		stored[i] = u.toMap()
	}

	if len(stored) == 0 {
		s.set(Users, nil)
	} else {
		s.set(Users, stored)
	}

	return s.writeNoLock()
}

// CreateUser adds a user. The first user must have the ADMIN role.
func (s *UserStore) CreateUser(ctx context.Context, u models.User, password string) error {
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}

	token, err := newSessionToken()
	if err != nil {
		return err
	}

	return s.modifyUsers(func(users []StoredUser) ([]StoredUser, error) {
		if s.findUser(users, u.Username) != -1 {
			return nil, user.ErrUserAlreadyExists
		}

		if len(users) == 0 && !u.Roles.HasRole(models.RoleEnumAdmin) {
			return nil, user.ErrFirstUserNotAdmin
		}

		if u.AutoLogin && otherAutoLoginHasNoPassword(users, -1) {
			return nil, user.ErrAutoLoginUserNoPassword
		}

		users = append(users, StoredUser{
			Username:     u.Username,
			PasswordHash: hash,
			Roles:        slices.Clone(u.Roles),
			SessionToken: token,
			AutoLogin:    u.AutoLogin,
		})

		if u.AutoLogin {
			clearOtherAutoLogin(users, len(users)-1)
		}

		return users, nil
	})
}

// ReplaceUser replaces the name, roles and auto login mark of the user, but not
// its password.
func (s *UserStore) ReplaceUser(ctx context.Context, username string, updated models.User) error {
	return s.modifyUsers(func(users []StoredUser) ([]StoredUser, error) {
		idx := s.findUser(users, username)
		if idx == -1 {
			return nil, user.ErrUserNotExist
		}

		if username != updated.Username && s.findUser(users, updated.Username) != -1 {
			return nil, user.ErrUserAlreadyExists
		}

		if users[idx].isAdmin() && !updated.Roles.HasRole(models.RoleEnumAdmin) && !hasOtherAdmin(users, username) {
			return nil, user.ErrRemoveLastAdminRole
		}

		if users[idx].AutoLogin && !users[idx].hasPassword() && !updated.AutoLogin {
			return nil, user.ErrAutoLoginNoPassword
		}

		if updated.AutoLogin && otherAutoLoginHasNoPassword(users, idx) {
			return nil, user.ErrAutoLoginUserNoPassword
		}

		users[idx].Username = updated.Username
		users[idx].Roles = slices.Clone(updated.Roles)
		users[idx].AutoLogin = updated.AutoLogin

		if updated.AutoLogin {
			clearOtherAutoLogin(users, idx)
		}

		return users, nil
	})
}

// ChangeUserPassword sets the password of the user, ending its sessions.
func (s *UserStore) ChangeUserPassword(ctx context.Context, username string, newPassword string) error {
	hash, err := hashPassword(newPassword)
	if err != nil {
		return err
	}

	token, err := newSessionToken()
	if err != nil {
		return err
	}

	return s.modifyUsers(func(users []StoredUser) ([]StoredUser, error) {
		idx := s.findUser(users, username)
		if idx == -1 {
			return nil, user.ErrUserNotExist
		}

		users[idx].PasswordHash = hash
		users[idx].SessionToken = token
		return users, nil
	})
}

// DeleteUser removes the user. The last admin user cannot be removed.
func (s *UserStore) DeleteUser(ctx context.Context, username string) error {
	return s.modifyUsers(func(users []StoredUser) ([]StoredUser, error) {
		idx := s.findUser(users, username)
		if idx == -1 {
			return nil, user.ErrUserNotExist
		}

		if users[idx].isAdmin() && !hasOtherAdmin(users, username) {
			return nil, user.ErrDeleteLastAdminUser
		}

		return slices.Delete(users, idx, idx+1), nil
	})
}

// CreateFirstAdmin creates an admin without a password and with auto login, if
// no users are configured. Returns true if the user was created.
func (s *UserStore) CreateFirstAdmin(ctx context.Context) (bool, error) {
	token, err := newSessionToken()
	if err != nil {
		return false, err
	}

	s.Lock()
	defer s.Unlock()

	users, err := s.allUsersNoLock()
	if err != nil {
		return false, err
	}

	if len(users) > 0 {
		return false, nil
	}

	admin := StoredUser{
		Username:     FirstAdminUsername,
		Roles:        []models.RoleEnum{models.RoleEnumAdmin},
		SessionToken: token,
		AutoLogin:    true,
	}

	s.set(Users, []map[string]interface{}{admin.toMap()})

	if err := s.writeNoLock(); err != nil {
		return false, err
	}

	return true, nil
}
