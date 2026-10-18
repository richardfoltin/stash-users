package manager

import (
	"context"
	"fmt"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/user"
)

// firstAdmin returns the name of the first user with the ADMIN role, or an
// empty string if there is none.
func firstAdmin(ctx context.Context, users user.UserSource) string {
	all, err := users.AllUsers(ctx)
	if err != nil {
		logger.Errorf("error getting the first admin: %v", err)
		return ""
	}

	for _, u := range all {
		if u.IsAdmin() {
			return u.Username
		}
	}

	return ""
}

type userActivityStore interface {
	Ready() error
	MoveUserActivity(ctx context.Context, from string, to *string) error
	CopyUserActivity(ctx context.Context, from string, to string) error
	AdoptLegacyActivity(ctx context.Context) error
}

// activityUserSource moves the viewing activity of renamed users, deletes that
// of removed users, and lets the first admin adopt the unowned activity.
type activityUserSource struct {
	user.UserSource
	activity userActivityStore
}

func (s *activityUserSource) CreateUser(ctx context.Context, u models.User, password string) error {
	if err := s.UserSource.CreateUser(ctx, u, password); err != nil {
		return err
	}

	return s.usersChanged(ctx, "", nil)
}

func (s *activityUserSource) ReplaceUser(ctx context.Context, username string, updated models.User) error {
	if err := s.UserSource.ReplaceUser(ctx, username, updated); err != nil {
		return err
	}

	if username == updated.Username {
		return s.usersChanged(ctx, "", nil)
	}

	return s.usersChanged(ctx, username, &updated.Username)
}

// CopyUserData copies the personal settings with the user source, and the
// viewing activity in the database.
func (s *activityUserSource) CopyUserData(ctx context.Context, from, to string, settings, activity bool) error {
	if err := s.UserSource.CopyUserData(ctx, from, to, settings, false); err != nil {
		return err
	}

	if !activity {
		return nil
	}

	if err := s.activity.Ready(); err != nil {
		return fmt.Errorf("copying the viewing activity of %q: %w", from, err)
	}

	return s.activity.CopyUserActivity(ctx, from, to)
}

func (s *activityUserSource) DeleteUser(ctx context.Context, username string) error {
	if err := s.UserSource.DeleteUser(ctx, username); err != nil {
		return err
	}

	return s.usersChanged(ctx, username, nil)
}

// usersChanged updates the activity of the user changed, who was renamed to
// renamedTo, or removed if renamedTo is nil.
func (s *activityUserSource) usersChanged(ctx context.Context, changed string, renamedTo *string) error {
	// the activity is updated when the database is opened
	if s.activity.Ready() != nil {
		return nil
	}

	if changed != "" {
		if err := s.activity.MoveUserActivity(ctx, changed, renamedTo); err != nil {
			return fmt.Errorf("moving the viewing activity of %q: %w", changed, err)
		}
	}

	return s.activity.AdoptLegacyActivity(ctx)
}
