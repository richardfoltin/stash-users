//go:build integration
// +build integration

package sqlite_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

// newPersonalFilterDB returns a new, empty database for the personal saved
// filter tests.
func newPersonalFilterDB(t *testing.T) *sqlite.Database {
	t.Helper()

	d := sqlite.NewDatabase()
	d.SetBlobStoreOptions(sqlite.BlobStoreOptions{UseDatabase: true})
	require.NoError(t, d.Open(filepath.Join(t.TempDir(), "personal-filters.sqlite")))
	t.Cleanup(func() { _ = d.Close() })

	return d
}

func personalFilter(mode models.FilterMode, name string) *models.SavedFilter {
	return &models.SavedFilter{
		Mode:         mode,
		Name:         name,
		ObjectFilter: map[string]interface{}{"title": map[string]interface{}{"value": name, "modifier": "EQUALS"}},
		UIOptions:    map[string]interface{}{"display_mode": float64(1)},
	}
}

// createPersonal creates the personal filter of the user, and returns it as
// stored.
func createPersonal(t *testing.T, d *sqlite.Database, user string, f *models.SavedFilter) *models.SavedFilter {
	t.Helper()

	require.NoError(t, txn.WithTxn(context.Background(), d, func(ctx context.Context) error {
		return d.PersonalSavedFilter.Create(ctx, user, f)
	}))

	return f
}

// personalNames returns the names of the personal filters of the user with
// the mode.
func personalNames(t *testing.T, d *sqlite.Database, user string, mode models.FilterMode) []string {
	t.Helper()

	var ret []string
	require.NoError(t, txn.WithReadTxn(context.Background(), d, func(ctx context.Context) error {
		filters, err := d.PersonalSavedFilter.FindByMode(ctx, user, mode)
		for _, f := range filters {
			ret = append(ret, f.Name)
		}
		return err
	}))

	return ret
}

func TestPersonalSavedFilterCreateAndFind(t *testing.T) {
	d := newPersonalFilterDB(t)
	f := createPersonal(t, d, "alice", personalFilter(models.FilterModeScenes, "Evening picks"))

	assert.True(t, models.IsPersonalSavedFilterID(f.ID), "id %d above the shared ids", f.ID)
	assert.True(t, f.Personal)
	assert.Equal(t, "Evening picks", f.Name)
	assert.Equal(t, models.FilterModeScenes, f.Mode)
	assert.Equal(t, map[string]interface{}{"display_mode": float64(1)}, f.UIOptions)

	require.NoError(t, txn.WithReadTxn(context.Background(), d, func(ctx context.Context) error {
		found, err := d.PersonalSavedFilter.Find(ctx, "alice", f.ID)
		require.NoError(t, err)
		assert.Equal(t, f, found, "owner")

		found, err = d.PersonalSavedFilter.Find(ctx, "bob", f.ID)
		require.NoError(t, err)
		assert.Nil(t, found, "another user")

		return nil
	}))
}

func TestPersonalSavedFilterIDsAboveShared(t *testing.T) {
	d := newPersonalFilterDB(t)
	personal := createPersonal(t, d, "alice", personalFilter(models.FilterModeScenes, "Mine"))

	shared := personalFilter(models.FilterModeScenes, "Ours")
	require.NoError(t, txn.WithTxn(context.Background(), d, func(ctx context.Context) error {
		return d.SavedFilter.Create(ctx, shared)
	}))

	assert.False(t, models.IsPersonalSavedFilterID(shared.ID), "shared id %d", shared.ID)
	assert.Equal(t, models.PersonalSavedFilterIDOffset+1, personal.ID, "first personal id")
}

func TestPersonalSavedFilterFindByMode(t *testing.T) {
	d := newPersonalFilterDB(t)
	createPersonal(t, d, "alice", personalFilter(models.FilterModeScenes, "b"))
	createPersonal(t, d, "alice", personalFilter(models.FilterModeScenes, "B"))
	createPersonal(t, d, "alice", personalFilter(models.FilterModeScenes, "a"))
	createPersonal(t, d, "alice", personalFilter(models.FilterModeImages, "image"))
	createPersonal(t, d, "alice", personalFilter(models.FilterModeMovies, "movie"))
	createPersonal(t, d, "bob", personalFilter(models.FilterModeScenes, "bob's"))

	assert.Equal(t, []string{"B", "a", "b"}, personalNames(t, d, "alice", models.FilterModeScenes), "own filters, capitals first")
	assert.Equal(t, []string{"movie"}, personalNames(t, d, "alice", models.FilterModeGroups), "groups include movies")
	assert.Equal(t, []string{"bob's"}, personalNames(t, d, "bob", models.FilterModeScenes))
	assert.Empty(t, personalNames(t, d, "carol", models.FilterModeScenes))

	require.NoError(t, txn.WithReadTxn(context.Background(), d, func(ctx context.Context) error {
		all, err := d.PersonalSavedFilter.All(ctx, "alice")
		assert.Len(t, all, 5)
		return err
	}))
}

func TestPersonalSavedFilterNamesUniquePerUserAndMode(t *testing.T) {
	d := newPersonalFilterDB(t)
	createPersonal(t, d, "alice", personalFilter(models.FilterModeScenes, "Favourites"))

	err := txn.WithTxn(context.Background(), d, func(ctx context.Context) error {
		return d.PersonalSavedFilter.Create(ctx, "alice", personalFilter(models.FilterModeScenes, "Favourites"))
	})
	assert.Error(t, err, "same user, mode and name")

	createPersonal(t, d, "bob", personalFilter(models.FilterModeScenes, "Favourites"))
	createPersonal(t, d, "alice", personalFilter(models.FilterModeImages, "Favourites"))
}

func TestPersonalSavedFilterUpdate(t *testing.T) {
	d := newPersonalFilterDB(t)
	f := createPersonal(t, d, "alice", personalFilter(models.FilterModeScenes, "Old"))

	renamed := *f
	renamed.Name = "New"
	err := txn.WithTxn(context.Background(), d, func(ctx context.Context) error {
		return d.PersonalSavedFilter.Update(ctx, "bob", &renamed)
	})
	assert.Error(t, err, "another user")
	assert.Equal(t, []string{"Old"}, personalNames(t, d, "alice", models.FilterModeScenes))

	require.NoError(t, txn.WithTxn(context.Background(), d, func(ctx context.Context) error {
		return d.PersonalSavedFilter.Update(ctx, "alice", &renamed)
	}))
	assert.Equal(t, []string{"New"}, personalNames(t, d, "alice", models.FilterModeScenes))
}

func TestPersonalSavedFilterDestroy(t *testing.T) {
	d := newPersonalFilterDB(t)
	f := createPersonal(t, d, "alice", personalFilter(models.FilterModeScenes, "Mine"))

	err := txn.WithTxn(context.Background(), d, func(ctx context.Context) error {
		return d.PersonalSavedFilter.Destroy(ctx, "bob", f.ID)
	})
	assert.Error(t, err, "another user")
	assert.Equal(t, []string{"Mine"}, personalNames(t, d, "alice", models.FilterModeScenes))

	require.NoError(t, txn.WithTxn(context.Background(), d, func(ctx context.Context) error {
		return d.PersonalSavedFilter.Destroy(ctx, "alice", f.ID)
	}))
	assert.Empty(t, personalNames(t, d, "alice", models.FilterModeScenes))
}

func TestMoveUserSavedFilters(t *testing.T) {
	d := newPersonalFilterDB(t)
	createPersonal(t, d, "alice", personalFilter(models.FilterModeScenes, "A"))
	createPersonal(t, d, "alice", personalFilter(models.FilterModeScenes, "B"))
	// left over from an earlier user with the new name
	leftover := personalFilter(models.FilterModeScenes, "A")
	leftover.UIOptions = map[string]interface{}{"display_mode": float64(2)}
	createPersonal(t, d, "alicia", leftover)
	createPersonal(t, d, "bob", personalFilter(models.FilterModeScenes, "C"))

	newName := "alicia"
	require.NoError(t, d.MoveUserSavedFilters(context.Background(), "alice", &newName))
	assert.Empty(t, personalNames(t, d, "alice", models.FilterModeScenes), "old name")
	assert.Equal(t, []string{"A", "B"}, personalNames(t, d, "alicia", models.FilterModeScenes), "new name")

	require.NoError(t, txn.WithReadTxn(context.Background(), d, func(ctx context.Context) error {
		filters, err := d.PersonalSavedFilter.FindByMode(ctx, "alicia", models.FilterModeScenes)
		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{"display_mode": float64(1)}, filters[0].UIOptions, "the renamed user's filter replaces the leftover")
		return nil
	}))

	require.NoError(t, d.MoveUserSavedFilters(context.Background(), "bob", nil))
	assert.Empty(t, personalNames(t, d, "bob", models.FilterModeScenes), "removed")
	assert.Equal(t, []string{"A", "B"}, personalNames(t, d, "alicia", models.FilterModeScenes), "the others keep theirs")
}

func TestAnonymiserDropsPersonalSavedFilters(t *testing.T) {
	d := newPersonalFilterDB(t)
	createPersonal(t, d, "alice", personalFilter(models.FilterModeScenes, "Mine"))

	out := filepath.Join(t.TempDir(), "anonymised.sqlite")
	anonymiser, err := sqlite.NewAnonymiser(d, out)
	require.NoError(t, err)
	require.NoError(t, anonymiser.Anonymise(context.Background()))

	a := sqlite.NewDatabase()
	a.SetBlobStoreOptions(sqlite.BlobStoreOptions{UseDatabase: true})
	require.NoError(t, a.Open(out))
	t.Cleanup(func() { _ = a.Close() })

	assert.Empty(t, personalNames(t, a, "alice", models.FilterModeScenes))
}
