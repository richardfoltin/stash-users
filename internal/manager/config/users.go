package config

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"

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

// errUnchanged is returned by a function that modifyUsers calls when it
// changed nothing, so that the config file is not written.
var errUnchanged = errors.New("unchanged")

// StoredUser is a user as stored in the config file, with its password hash.
type StoredUser struct {
	Username     string `json:"username" koanf:"username"`
	PasswordHash string `json:"passwordhash" koanf:"passwordhash"`
	// Roles are the IDs of the role of the user, empty if the user is locked
	Roles []string `json:"roles" koanf:"roles"`
	// SessionToken is replaced when the password changes, ending the sessions
	SessionToken string `json:"sessiontoken" koanf:"sessiontoken"`
	AutoLogin    bool   `json:"autologin" koanf:"autologin"`
	// Interface and UI are the units of the interface and UI configuration
	// that the user changed for themselves
	Interface map[string]interface{} `json:"interface,omitempty" koanf:"interface"`
	UI        map[string]interface{} `json:"ui,omitempty" koanf:"ui"`
}

func (u StoredUser) toMap() map[string]interface{} {
	roles := make([]string, len(u.Roles))
	copy(roles, u.Roles)

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

	if len(u.Interface) > 0 {
		ret["interface"] = u.Interface
	}

	if len(u.UI) > 0 {
		ret["ui"] = u.UI
	}

	return ret
}

// toUser returns the user with the permissions of its role: the Admin role if
// the user has it, otherwise the first of its roles that exists. A user stored
// with several roles by an earlier version has the highest of the built-in
// roles, as then. A user without an existing role is locked.
func (u StoredUser) toUser(roles []StoredRole) *models.User {
	ret := &models.User{
		Username:    u.Username,
		AutoLogin:   u.AutoLogin,
		HasPassword: u.hasPassword(),
	}

	if u.isAdmin() {
		ret.Role = models.RoleIDAdmin
		return ret
	}

	ids := u.Roles
	if slices.Contains(ids, models.RoleIDEditor) {
		ids = append([]string{models.RoleIDEditor}, ids...)
	}

	for _, id := range ids {
		if idx := findRole(roles, id); idx != -1 {
			ret.Role = id
			ret.Permissions = roles[idx].validPermissions()
			break
		}
	}

	return ret
}

func (u StoredUser) hasPassword() bool {
	return u.PasswordHash != ""
}

func (u StoredUser) isAdmin() bool {
	return slices.Contains(u.Roles, models.RoleIDAdmin)
}

// storedRoleIDs returns the role IDs to store for the role: none for a locked
// user.
func storedRoleIDs(role string) []string {
	if role == "" {
		return nil
	}

	return []string{role}
}

// checkRoleExists returns ErrRoleNotExist unless the role is empty, Admin or
// one of the roles.
func checkRoleExists(roles []StoredRole, role string) error {
	if role == "" || role == models.RoleIDAdmin || findRole(roles, role) != -1 {
		return nil
	}

	return user.ErrRoleNotExist
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

	// the users and roles lists, which every request reads, as last decoded
	users atomic.Pointer[decoded[[]StoredUser]]
	roles atomic.Pointer[decoded[[]StoredRole]]
}

// decoded is a value decoded from the configuration, with the generation of
// the configuration it was decoded from.
type decoded[T any] struct {
	generation uint64
	value      T
}

// cloneUsers returns copies of the users that share nothing with them.
func cloneUsers(users []StoredUser) []StoredUser {
	ret := make([]StoredUser, len(users))
	for i, u := range users {
		u.Roles = slices.Clone(u.Roles)
		if u.Interface != nil {
			u.Interface = copyMap(u.Interface)
		}
		if u.UI != nil {
			u.UI = copyMap(u.UI)
		}
		ret[i] = u
	}

	return ret
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
		Roles:        []string{models.RoleIDAdmin},
	}
}

// listedUsersNoLock returns the users list. Every request reads it, so it is
// decoded once for each change of the configuration, and the users are shared:
// they must not be changed. Assumes read lock held.
func (s *UserStore) listedUsersNoLock() ([]StoredUser, error) {
	if c := s.users.Load(); c != nil && c.generation == s.generation {
		return c.value, nil
	}

	var ret []StoredUser
	if err := s.forKey(Users).Unmarshal(Users, &ret); err != nil {
		return nil, fmt.Errorf("reading users from config: %w", err)
	}

	s.users.Store(&decoded[[]StoredUser]{generation: s.generation, value: ret})
	return ret, nil
}

// allUsersNoLock returns all users, including the legacy user. The users are
// shared, and must not be changed. Assumes read lock held.
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

	u := users[idx]
	return &u, nil
}

// GetUser returns the user with the provided username, or nil if not found.
func (s *UserStore) GetUser(ctx context.Context, username string) (*models.User, error) {
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

	roles, err := s.rolesNoLock()
	if err != nil {
		return nil, err
	}

	return users[idx].toUser(roles), nil
}

// AllUsers returns all users, including the legacy user.
func (s *UserStore) AllUsers(ctx context.Context) ([]*models.User, error) {
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

	ret := make([]*models.User, len(users))
	for i, u := range users {
		ret[i] = u.toUser(roles)
	}

	return ret, nil
}

// LoginRequired returns true if any users are configured.
// It is called for every request.
func (s *UserStore) LoginRequired(ctx context.Context) bool {
	s.RLock()
	defer s.RUnlock()

	if s.legacyUserNoLock() != nil {
		return true
	}

	listed, err := s.listedUsersNoLock()

	// fail closed - an unexpected value must not disable authentication
	return err != nil || len(listed) > 0
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

	// fn changes the users, which must not be the shared ones
	users, err = fn(cloneUsers(users))
	if errors.Is(err, errUnchanged) {
		return nil
	}
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

// CreateUser adds a user. The role must exist, and the first user must have
// the Admin role.
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

		roles, err := s.rolesNoLock()
		if err != nil {
			return nil, err
		}

		if err := checkRoleExists(roles, u.Role); err != nil {
			return nil, err
		}

		if len(users) == 0 && !u.IsAdmin() {
			return nil, user.ErrFirstUserNotAdmin
		}

		if u.AutoLogin && otherAutoLoginHasNoPassword(users, -1) {
			return nil, user.ErrAutoLoginUserNoPassword
		}

		users = append(users, StoredUser{
			Username:     u.Username,
			PasswordHash: hash,
			Roles:        storedRoleIDs(u.Role),
			SessionToken: token,
			AutoLogin:    u.AutoLogin,
		})

		if u.AutoLogin {
			clearOtherAutoLogin(users, len(users)-1)
		}

		return users, nil
	})
}

// ReplaceUser replaces the name, role and auto login mark of the user, but not
// its password. The role must exist.
func (s *UserStore) ReplaceUser(ctx context.Context, username string, updated models.User) error {
	return s.modifyUsers(func(users []StoredUser) ([]StoredUser, error) {
		idx := s.findUser(users, username)
		if idx == -1 {
			return nil, user.ErrUserNotExist
		}

		if username != updated.Username && s.findUser(users, updated.Username) != -1 {
			return nil, user.ErrUserAlreadyExists
		}

		roles, err := s.rolesNoLock()
		if err != nil {
			return nil, err
		}

		if err := checkRoleExists(roles, updated.Role); err != nil {
			return nil, err
		}

		if users[idx].isAdmin() && !updated.IsAdmin() && !hasOtherAdmin(users, username) {
			return nil, user.ErrRemoveLastAdminRole
		}

		if users[idx].AutoLogin && !users[idx].hasPassword() && !updated.AutoLogin {
			return nil, user.ErrAutoLoginNoPassword
		}

		if updated.AutoLogin && otherAutoLoginHasNoPassword(users, idx) {
			return nil, user.ErrAutoLoginUserNoPassword
		}

		users[idx].Username = updated.Username
		users[idx].Roles = storedRoleIDs(updated.Role)
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
		Roles:        []string{models.RoleIDAdmin},
		SessionToken: token,
		AutoLogin:    true,
	}

	s.set(Users, []map[string]interface{}{admin.toMap()})

	if err := s.writeNoLock(); err != nil {
		return false, err
	}

	return true, nil
}
