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
	ErrInvalidRole              = errors.New("invalid role")
	ErrFirstUserNotAdmin        = errors.New("the first user must be an admin")
	ErrDeleteLastAdminUser      = errors.New("final admin user cannot be deleted")
	ErrRemoveLastAdminRole      = errors.New("final admin role cannot be removed")
	ErrInternalError            = errors.New("internal error")
	ErrAccessDenied             = errors.New("access denied")
	ErrCurrentPasswordIncorrect = errors.New("current password incorrect")
	ErrUserAlreadyExists        = errors.New("user with that username already exists")
)

// UserSource stores the users. It keeps the usernames unique, and the first
// and the last user admins.
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
	return len(u.Roles) == 0
}

func (s *Service) ValidateCredentials(ctx context.Context, username string, password string) error {
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
			Roles:    models.Roles{models.RoleEnumAdmin},
		}, nil
	}

	u, err := s.GetUser(ctx, username)
	if err != nil {
		logger.Errorf("error getting api key user: %v", err)
		return nil, ErrInternalError
	}

	if u == nil || !u.Roles.HasRole(models.RoleEnumAdmin) {
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

func (s *Service) validateRoles(roles models.Roles) error {
	for _, r := range roles {
		if !r.IsValid() {
			return fmt.Errorf("%w: %q", ErrInvalidRole, r)
		}
	}

	return nil
}

func (s *Service) CreateUser(ctx context.Context, u models.User, password string) error {
	// validate input
	// ensure username is valid
	if err := s.validateUsername(u.Username); err != nil {
		return err
	}

	if err := s.validateRoles(u.Roles); err != nil {
		return err
	}

	// validate password
	if err := ValidatePassword(password); err != nil {
		return err
	}

	// create user in store - the store checks that the username is unique and
	// that the first user is an admin
	if err := s.Store.CreateUser(ctx, u, password); err != nil {
		return err
	}

	logger.Infof("[user] created %q", u.Username)

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

	if err := s.validateRoles(updated.Roles); err != nil {
		return err
	}

	// ensure username is valid
	if username != updated.Username {
		if err := s.validateUsername(updated.Username); err != nil {
			return err
		}
	}

	// update user in store - the store checks that the new username is
	// unique and that the admin role is not removed from the last admin
	if err := s.Store.ReplaceUser(ctx, username, updated); err != nil {
		return err
	}

	if username != updated.Username {
		logger.Infof("[user] updated name %q -> %q", username, updated.Username)
	}

	if !slices.Equal(existingUser.Roles, updated.Roles) {
		logger.Infof("[user] updated roles for user %q", updated.Username)
	}

	return nil
}

// ChangePassword changes the password of the user, checking the current
// password.
func (s *Service) ChangePassword(ctx context.Context, username, currentPassword, newPassword string) error {
	// validate current credentials
	if err := s.ValidateCredentials(ctx, username, currentPassword); err != nil {
		logger.Infof("[user] failed password change attempt for %q: incorrect current password", username)
		return ErrCurrentPasswordIncorrect
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
