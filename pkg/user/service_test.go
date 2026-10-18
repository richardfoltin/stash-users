package user

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/stashapp/stash/pkg/models"
)

type fakeStore struct {
	UserSource
	users   map[string]bool
	copyErr error
	copied  []CopyFrom
}

func (s *fakeStore) GetUser(ctx context.Context, username string) (*models.User, error) {
	if !s.users[username] {
		return nil, nil
	}

	return &models.User{Username: username}, nil
}

func (s *fakeStore) CreateUser(ctx context.Context, u models.User, password string) error {
	s.users[u.Username] = true
	return nil
}

func (s *fakeStore) DeleteUser(ctx context.Context, username string) error {
	delete(s.users, username)
	return nil
}

func (s *fakeStore) CopyUserData(ctx context.Context, from, to string, settings, activity bool) error {
	s.copied = append(s.copied, CopyFrom{Username: from + ">" + to, Settings: settings, Activity: activity})
	return s.copyErr
}

func TestCreateUserFrom(t *testing.T) {
	ctx := context.Background()
	newUser := models.User{Username: "bela", Role: models.RoleIDViewer}

	t.Run("copies", func(t *testing.T) {
		store := &fakeStore{users: map[string]bool{"anna": true}}
		s := &Service{Store: store}

		assert.NoError(t, s.CreateUserFrom(ctx, newUser, "password", CopyFrom{Username: "anna", Settings: true, Activity: true}))
		assert.True(t, store.users["bela"])
		assert.Equal(t, []CopyFrom{{Username: "anna>bela", Settings: true, Activity: true}}, store.copied)
	})

	t.Run("nothing to copy", func(t *testing.T) {
		store := &fakeStore{users: map[string]bool{"anna": true}}
		s := &Service{Store: store}

		assert.NoError(t, s.CreateUserFrom(ctx, newUser, "password", CopyFrom{Username: "anna"}))
		assert.True(t, store.users["bela"])
		assert.Empty(t, store.copied)
	})

	t.Run("unknown source", func(t *testing.T) {
		store := &fakeStore{users: map[string]bool{}}
		s := &Service{Store: store}

		assert.ErrorIs(t, s.CreateUserFrom(ctx, newUser, "password", CopyFrom{Username: "anna", Settings: true}), ErrUserNotExist)
		assert.False(t, store.users["bela"], "not created")
	})

	t.Run("a failed copy deletes the user", func(t *testing.T) {
		copyErr := errors.New("database closed")
		store := &fakeStore{users: map[string]bool{"anna": true}, copyErr: copyErr}
		s := &Service{Store: store}

		assert.ErrorIs(t, s.CreateUserFrom(ctx, newUser, "password", CopyFrom{Username: "anna", Activity: true}), copyErr)
		assert.False(t, store.users["bela"], "deleted again")
		assert.True(t, store.users["anna"])
	})
}
