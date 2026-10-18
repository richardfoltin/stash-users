package manager

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/user"
)

type fakeUserSource struct {
	user.UserSource
	users  []*models.User
	copies []copyCall
	err    error
}

func (s *fakeUserSource) AllUsers(ctx context.Context) ([]*models.User, error) {
	return s.users, s.err
}

func (s *fakeUserSource) CreateUser(ctx context.Context, u models.User, password string) error {
	return s.err
}

func (s *fakeUserSource) ReplaceUser(ctx context.Context, username string, updated models.User) error {
	return s.err
}

func (s *fakeUserSource) DeleteUser(ctx context.Context, username string) error {
	return s.err
}

type copyCall struct {
	from, to           string
	settings, activity bool
}

func (s *fakeUserSource) CopyUserData(ctx context.Context, from, to string, settings, activity bool) error {
	s.copies = append(s.copies, copyCall{from, to, settings, activity})
	return s.err
}

type activityMove struct {
	from string
	to   *string
}

type fakeActivityStore struct {
	notReady bool
	moves    []activityMove
	copies   []activityMove
	adopted  int
}

func (s *fakeActivityStore) Ready() error {
	if s.notReady {
		return errors.New("not ready")
	}
	return nil
}

func (s *fakeActivityStore) MoveUserActivity(ctx context.Context, from string, to *string) error {
	s.moves = append(s.moves, activityMove{from, to})
	return nil
}

func (s *fakeActivityStore) CopyUserActivity(ctx context.Context, from string, to string) error {
	s.copies = append(s.copies, activityMove{from, &to})
	return nil
}

func (s *fakeActivityStore) AdoptLegacyActivity(ctx context.Context) error {
	s.adopted++
	return nil
}

func TestFirstAdmin(t *testing.T) {
	read := models.RoleIDViewer
	admin := models.RoleIDAdmin

	assert.Equal(t, "", firstAdmin(context.Background(), &fakeUserSource{}), "no users")
	assert.Equal(t, "", firstAdmin(context.Background(), &fakeUserSource{users: []*models.User{{Username: "viewer", Role: read}}}), "no admin")
	assert.Equal(t, "second", firstAdmin(context.Background(), &fakeUserSource{users: []*models.User{
		{Username: "viewer", Role: read},
		{Username: "second", Role: admin},
		{Username: "third", Role: admin},
	}}), "config order")
	assert.Equal(t, "", firstAdmin(context.Background(), &fakeUserSource{err: errors.New("broken config")}), "error")
}

func TestActivityUserSource(t *testing.T) {
	ctx := context.Background()
	newName := "alicia"

	t.Run("rename moves the activity", func(t *testing.T) {
		store := &fakeActivityStore{}
		s := &activityUserSource{UserSource: &fakeUserSource{}, activity: store}

		assert.NoError(t, s.ReplaceUser(ctx, "alice", models.User{Username: newName}))
		assert.Equal(t, []activityMove{{"alice", &newName}}, store.moves)
		assert.Equal(t, 1, store.adopted)
	})

	t.Run("role change keeps the activity", func(t *testing.T) {
		store := &fakeActivityStore{}
		s := &activityUserSource{UserSource: &fakeUserSource{}, activity: store}

		assert.NoError(t, s.ReplaceUser(ctx, "alice", models.User{Username: "alice", Role: models.RoleIDAdmin}))
		assert.Empty(t, store.moves)
	})

	t.Run("removal deletes the activity", func(t *testing.T) {
		store := &fakeActivityStore{}
		s := &activityUserSource{UserSource: &fakeUserSource{}, activity: store}

		assert.NoError(t, s.DeleteUser(ctx, "bob"))
		assert.Equal(t, []activityMove{{"bob", nil}}, store.moves)
	})

	t.Run("a new user adopts the legacy activity if first admin", func(t *testing.T) {
		store := &fakeActivityStore{}
		s := &activityUserSource{UserSource: &fakeUserSource{}, activity: store}

		assert.NoError(t, s.CreateUser(ctx, models.User{Username: "admin"}, "password"))
		assert.Empty(t, store.moves)
		assert.Equal(t, 1, store.adopted)
	})

	t.Run("a failed change keeps the activity", func(t *testing.T) {
		store := &fakeActivityStore{}
		s := &activityUserSource{UserSource: &fakeUserSource{err: user.ErrDeleteLastAdminUser}, activity: store}

		assert.ErrorIs(t, s.DeleteUser(ctx, "admin"), user.ErrDeleteLastAdminUser)
		assert.ErrorIs(t, s.ReplaceUser(ctx, "admin", models.User{Username: newName}), user.ErrDeleteLastAdminUser)
		assert.Empty(t, store.moves)
		assert.Zero(t, store.adopted)
	})

	t.Run("no database", func(t *testing.T) {
		store := &fakeActivityStore{notReady: true}
		s := &activityUserSource{UserSource: &fakeUserSource{}, activity: store}

		assert.NoError(t, s.DeleteUser(ctx, "bob"))
		assert.Empty(t, store.moves)
	})
}

func TestActivityUserSourceCopy(t *testing.T) {
	ctx := context.Background()
	bela := "bela"

	t.Run("settings and activity", func(t *testing.T) {
		store := &fakeActivityStore{}
		users := &fakeUserSource{}
		s := &activityUserSource{UserSource: users, activity: store}

		assert.NoError(t, s.CopyUserData(ctx, "anna", bela, true, true))
		assert.Equal(t, []copyCall{{"anna", bela, true, false}}, users.copies, "the user source copies the settings")
		assert.Equal(t, []activityMove{{"anna", &bela}}, store.copies)
	})

	t.Run("settings only", func(t *testing.T) {
		store := &fakeActivityStore{}
		s := &activityUserSource{UserSource: &fakeUserSource{}, activity: store}

		assert.NoError(t, s.CopyUserData(ctx, "anna", bela, true, false))
		assert.Empty(t, store.copies)
	})

	t.Run("activity without a database", func(t *testing.T) {
		store := &fakeActivityStore{notReady: true}
		s := &activityUserSource{UserSource: &fakeUserSource{}, activity: store}

		assert.Error(t, s.CopyUserData(ctx, "anna", bela, false, true))
		assert.Empty(t, store.copies)
	})

	t.Run("failed settings copy", func(t *testing.T) {
		store := &fakeActivityStore{}
		s := &activityUserSource{UserSource: &fakeUserSource{err: user.ErrUserNotExist}, activity: store}

		assert.ErrorIs(t, s.CopyUserData(ctx, "anna", bela, true, true), user.ErrUserNotExist)
		assert.Empty(t, store.copies)
	})
}
