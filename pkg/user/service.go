// Package user provides user management and authentication.
package user

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
)

// MaxPasswordLength is the maximum length of a password in bytes.
// bcrypt cannot hash longer passwords.
const MaxPasswordLength = 72

var (
	ErrUserNotExist             = errors.New("user not found")
	ErrEmptyUsername            = errors.New("empty username")
	ErrUsernameHasWhitespace    = errors.New("username has leading or trailing whitespace")
	ErrEmptyPassword            = errors.New("password cannot be empty")
	ErrPasswordTooLong          = errors.New("password cannot be longer than 72 bytes")
	ErrFirstUserNotAdmin        = errors.New("the first user must be an admin")
	ErrDeleteLastAdminUser      = errors.New("final admin user cannot be deleted")
	ErrRemoveLastAdminRole      = errors.New("final admin role cannot be removed")
	ErrInternalError            = errors.New("internal error")
	ErrAccessDenied             = errors.New("access denied")
	ErrCurrentPasswordIncorrect = errors.New("current password incorrect")
	ErrUserAlreadyExists        = errors.New("user with that username already exists")
	ErrAutoLoginNoPassword      = errors.New("auto login cannot be turned off for a user without a password: set a password first")
	ErrAutoLoginUserNoPassword  = errors.New("auto login cannot be given to another user while the auto login user has no password: set a password for that user first")
	ErrRoleNotExist             = errors.New("role does not exist")
	ErrRoleInUse                = errors.New("the role is assigned to users: give them another role first")
	ErrBuiltinRole              = errors.New("the Admin role cannot be changed or removed")
	ErrRoleNameExists           = errors.New("a role with that name already exists")
	ErrEmptyRoleName            = errors.New("role name cannot be empty")
	ErrInvalidPermission        = errors.New("invalid permission")
)

// UserSource stores the users. It keeps the usernames unique, the first and
// the last user admins, and at most one user marked for auto login, who keeps
// the mark while it has no password.
type UserSource interface {
	AllUsers(ctx context.Context) ([]*models.User, error)
	GetUser(ctx context.Context, username string) (*models.User, error)
	ValidateCredentials(ctx context.Context, username string, password string) bool
	LoginRequired(ctx context.Context) bool

	// GetSessionToken returns the token that the sessions of the user must
	// contain. It changes with the password.
	GetSessionToken(ctx context.Context, username string) (string, error)

	CreateUser(ctx context.Context, u models.User, password string) error
	ReplaceUser(ctx context.Context, username string, updated models.User) error
	ChangeUserPassword(ctx context.Context, username string, newPassword string) error
	DeleteUser(ctx context.Context, username string) error
	// CopyUserData copies the personal settings and/or the viewing activity
	// of the user from to the user to.
	CopyUserData(ctx context.Context, from, to string, settings, activity bool) error

	// AllRoles returns the built-in Admin role, followed by the other roles.
	AllRoles(ctx context.Context) ([]*models.Role, error)
	// GetRole returns the role with the ID, or nil if it does not exist.
	GetRole(ctx context.Context, id string) (*models.Role, error)
	// CreateRole adds the role with a new ID, and returns it. Role names are
	// unique, ignoring case.
	CreateRole(ctx context.Context, r models.Role) (*models.Role, error)
	UpdateRole(ctx context.Context, r models.Role) error
	// DeleteRole removes the role, unless a user has it.
	DeleteRole(ctx context.Context, id string) error
	// RoleUserCounts returns the number of users of each role that has users.
	RoleUserCounts(ctx context.Context) (map[string]int, error)

	// GetAPIKey returns the global API key.
	GetAPIKey() string
}

type Service struct {
	Store UserSource
	// APIKeyUserID returns the user ID of a valid API key.
	APIKeyUserID func(apiKey string) (string, error)
}

// LoginRequired returns true if any users are configured.
func (s *Service) LoginRequired(ctx context.Context) bool {
	return s.Store.LoginRequired(ctx)
}

func (s *Service) GetUser(ctx context.Context, username string) (*models.User, error) {
	return s.Store.GetUser(ctx, username)
}

func (s *Service) AllUsers(ctx context.Context) ([]*models.User, error) {
	return s.Store.AllUsers(ctx)
}

func userIsLocked(u *models.User) bool {
	return u.IsLocked()
}

func (s *Service) ValidateCredentials(ctx context.Context, username string, password string) error {
	// an empty password is never accepted - a user without a password can
	// only be logged in automatically
	if password == "" {
		logger.Infof("[login attempt] empty password for user %s", username)
		return ErrAccessDenied
	}

	// ensure user is not locked
	u, err := s.GetUser(ctx, username)
	if err != nil {
		logger.Errorf("error getting user for credential validation: %v", err)
		return ErrInternalError
	}

	if u == nil {
		logger.Infof("[login attempt] user %s not found during credential validation", username)
		return ErrAccessDenied
	}

	if userIsLocked(u) {
		logger.Infof("[login attempt] user %s is locked", username)
		return ErrAccessDenied
	}

	if !s.Store.ValidateCredentials(ctx, username, password) {
		logger.Infof("[login attempt] invalid credentials for user %s", username)
		return ErrAccessDenied
	}
	return nil
}

// Login validates the credentials and returns the token to store in the
// session of the user.
func (s *Service) Login(ctx context.Context, username string, password string) (string, error) {
	if err := s.ValidateCredentials(ctx, username, password); err != nil {
		return "", err
	}

	token, err := s.Store.GetSessionToken(ctx, username)
	if err != nil {
		logger.Errorf("error getting session token: %v", err)
		return "", ErrInternalError
	}

	return token, nil
}

// AuthenticateSession returns the user of a session, or ErrAccessDenied if the
// user does not exist, is locked, or the session token does not match.
func (s *Service) AuthenticateSession(ctx context.Context, username string, sessionToken string) (*models.User, error) {
	u, err := s.GetUser(ctx, username)
	if err != nil {
		logger.Errorf("error getting user for authentication: %v", err)
		return nil, ErrInternalError
	}

	if u == nil {
		logger.Infof("[authentication] user %s not found", username)
		return nil, ErrAccessDenied
	}

	if userIsLocked(u) {
		logger.Infof("[authentication] user %s is locked", username)
		return nil, ErrAccessDenied
	}

	token, err := s.Store.GetSessionToken(ctx, username)
	if err != nil {
		logger.Errorf("error getting session token: %v", err)
		return nil, ErrInternalError
	}

	if subtle.ConstantTimeCompare([]byte(sessionToken), []byte(token)) != 1 {
		logger.Infof("[authentication] session of user %s is no longer valid", username)
		return nil, ErrAccessDenied
	}

	return u, nil
}

// AutoLoginUser returns the unlocked user marked for auto login, if any, and
// the token to store in its session.
func (s *Service) AutoLoginUser(ctx context.Context) (*models.User, string, error) {
	users, err := s.Store.AllUsers(ctx)
	if err != nil {
		logger.Errorf("error getting users for auto login: %v", err)
		return nil, "", ErrInternalError
	}

	idx := slices.IndexFunc(users, func(u *models.User) bool {
		return u.AutoLogin
	})
	if idx == -1 || userIsLocked(users[idx]) {
		return nil, "", nil
	}

	u := users[idx]
	token, err := s.Store.GetSessionToken(ctx, u.Username)
	if errors.Is(err, ErrUserNotExist) {
		// the user was removed concurrently
		return nil, "", nil
	}
	if err != nil {
		logger.Errorf("error getting session token: %v", err)
		return nil, "", ErrInternalError
	}

	return u, token, nil
}

// AuthenticateByAPIKey authenticates the global API key as the admin it was
// generated for, or with the ADMIN role if no users are configured.
func (s *Service) AuthenticateByAPIKey(ctx context.Context, apiKey string) (*models.User, error) {
	globalKey := s.Store.GetAPIKey()
	if globalKey == "" || subtle.ConstantTimeCompare([]byte(apiKey), []byte(globalKey)) != 1 {
		logger.Infof("[apikey authentication] invalid api key")
		return nil, ErrAccessDenied
	}

	// a key without a valid user ID authenticates only without users
	username, _ := s.APIKeyUserID(globalKey)

	if !s.LoginRequired(ctx) {
		return &models.User{
			Username: username,
			Role:     models.RoleIDAdmin,
		}, nil
	}

	u, err := s.GetUser(ctx, username)
	if err != nil {
		logger.Errorf("error getting api key user: %v", err)
		return nil, ErrInternalError
	}

	if u == nil || !u.IsAdmin() {
		logger.Infof("[apikey authentication] api key user %q does not exist or is not an admin", username)
		return nil, ErrAccessDenied
	}

	return u, nil
}

func (s *Service) validateUsername(username string) error {
	if username == "" {
		return ErrEmptyUsername
	}

	// username must not have leading or trailing whitespace
	trimmed := strings.TrimSpace(username)

	if trimmed != username {
		return ErrUsernameHasWhitespace
	}

	return nil
}

// ValidatePassword returns an error if the password cannot be used.
func ValidatePassword(password string) error {
	if password == "" {
		return ErrEmptyPassword
	}

	if len(password) > MaxPasswordLength {
		return ErrPasswordTooLong
	}

	return nil
}

func (s *Service) CreateUser(ctx context.Context, u models.User, password string) error {
	// validate input
	// ensure username is valid
	if err := s.validateUsername(u.Username); err != nil {
		return err
	}

	// validate password
	if err := ValidatePassword(password); err != nil {
		return err
	}

	// create user in store - the store checks that the username is unique,
	// that the role exists and that the first user is an admin
	if err := s.Store.CreateUser(ctx, u, password); err != nil {
		return err
	}

	logger.Infof("[user] created %q", u.Username)

	return nil
}

// CopyFrom selects what a new user copies from an existing user.
type CopyFrom struct {
	Username string
	// Settings copies the personal interface settings
	Settings bool
	// Activity copies the plays, play history, O counts and resume positions
	Activity bool
}

// CreateUserFrom creates a user with a copy of the settings or the viewing
// activity of an existing user. If copying fails, the user is deleted again.
func (s *Service) CreateUserFrom(ctx context.Context, u models.User, password string, from CopyFrom) error {
	source, err := s.GetUser(ctx, from.Username)
	if err != nil {
		return err
	}

	if source == nil {
		return ErrUserNotExist
	}

	if err := s.CreateUser(ctx, u, password); err != nil {
		return err
	}

	if !from.Settings && !from.Activity {
		return nil
	}

	if err := s.Store.CopyUserData(ctx, from.Username, u.Username, from.Settings, from.Activity); err != nil {
		if delErr := s.Store.DeleteUser(ctx, u.Username); delErr != nil {
			logger.Errorf("[user] deleting %q after the failed copy: %v", u.Username, delErr)
		}

		return fmt.Errorf("copying from %q: %w", from.Username, err)
	}

	logger.Infof("[user] copied %q to %q", from.Username, u.Username)

	return nil
}

func (s *Service) UpdateUser(ctx context.Context, username string, updated models.User) error {
	// validate input
	// check if user exists
	existingUser, err := s.GetUser(ctx, username)
	if err != nil {
		return fmt.Errorf("error getting existing user: %w", err)
	}

	if existingUser == nil {
		return ErrUserNotExist
	}

	// ensure username is valid
	if username != updated.Username {
		if err := s.validateUsername(updated.Username); err != nil {
			return err
		}
	}

	// update user in store - the store checks that the new username is
	// unique, that the role exists and that the admin role is not removed
	// from the last admin
	if err := s.Store.ReplaceUser(ctx, username, updated); err != nil {
		return err
	}

	if username != updated.Username {
		logger.Infof("[user] updated name %q -> %q", username, updated.Username)
	}

	if existingUser.Role != updated.Role {
		logger.Infof("[user] updated role for user %q", updated.Username)
	}

	if existingUser.AutoLogin != updated.AutoLogin {
		logger.Infof("[user] set auto login for user %q to %t", updated.Username, updated.AutoLogin)
	}

	return nil
}

// ChangePassword changes the password of the user, checking the current
// password. A user without a password sets the first one without it.
func (s *Service) ChangePassword(ctx context.Context, username, currentPassword, newPassword string) error {
	u, err := s.GetUser(ctx, username)
	if err != nil {
		logger.Errorf("error getting user for password change: %v", err)
		return ErrInternalError
	}

	// validate current credentials
	if u == nil || u.HasPassword {
		if err := s.ValidateCredentials(ctx, username, currentPassword); err != nil {
			logger.Infof("[user] failed password change attempt for %q: incorrect current password", username)
			return ErrCurrentPasswordIncorrect
		}
	}

	return s.ChangeUserPassword(ctx, username, newPassword)
}

// ChangeUserPassword sets the password of the user. All existing sessions of
// the user end.
func (s *Service) ChangeUserPassword(ctx context.Context, username, newPassword string) error {
	// validate new password
	if err := ValidatePassword(newPassword); err != nil {
		return err
	}

	// change password in store
	if err := s.Store.ChangeUserPassword(ctx, username, newPassword); err != nil {
		return err
	}

	logger.Infof("[user] changed password for %q", username)

	return nil
}

func (s *Service) DeleteUser(ctx context.Context, username string) error {
	// delete user from store - the store checks that the last admin user is
	// not deleted
	if err := s.Store.DeleteUser(ctx, username); err != nil {
		return err
	}

	logger.Infof("[user] deleted %q", username)

	return nil
}
