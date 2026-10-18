//go:build integration
// +build integration

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// savedFilterTest runs saved filter operations against a new database.
type savedFilterTest struct {
	t    *testing.T
	exec *executor.Executor
}

func newSavedFilterTest(t *testing.T) *savedFilterTest {
	t.Helper()

	cfg := config.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))

	db := sqlite.NewDatabase()
	db.SetBlobStoreOptions(sqlite.BlobStoreOptions{UseDatabase: true})
	require.NoError(t, db.Open(filepath.Join(t.TempDir(), "stash-go.sqlite")))
	t.Cleanup(func() { _ = db.Close() })

	exec := executor.New(NewExecutableSchema(Config{
		Resolvers: &Resolver{repository: db.Repository()},
		Directives: DirectiveRoot{
			HasPermission: HasPermissionDirective,
			AdminOnly:     AdminOnlyDirective,
			SignedIn:      SignedInDirective,
			IsUserOwner:   IsUserOwnerDirective,
		},
	}))
	exec.AroundRootFields(mutationRequirementMiddleware)

	return &savedFilterTest{t: t, exec: exec}
}

// savedFilterUser returns a user with the name and the role with the ID:
// Admin or a default role.
func savedFilterUser(name string, roleID string) *models.User {
	u := models.User{Username: name, Role: roleID}
	for _, r := range models.DefaultRoles() {
		if r.ID == roleID {
			u.Permissions = r.Permissions
		}
	}

	return &u
}

// run executes the operation as the user, or without a user if u is nil, and
// returns the response data and errors.
func (s *savedFilterTest) run(u *models.User, query string, args ...interface{}) (map[string]interface{}, []error) {
	s.t.Helper()

	ctx := context.Background()
	if u != nil {
		ctx = models.SetCurrentUser(ctx, *u)
	}
	ctx = graphql.StartOperationTrace(ctx)

	opCtx, errs := s.exec.CreateOperationContext(ctx, &graphql.RawParams{Query: fmt.Sprintf(query, args...)})
	require.Empty(s.t, errs)

	handler, ctx := s.exec.DispatchOperation(ctx, opCtx)
	resp := handler(ctx)
	require.NotNil(s.t, resp)

	var data map[string]interface{}
	if len(resp.Data) > 0 {
		require.NoError(s.t, json.Unmarshal(resp.Data, &data))
	}

	var retErrs []error
	for _, err := range resp.Errors {
		retErrs = append(retErrs, err)
	}

	return data, retErrs
}

// save saves a scene filter as the user, overwriting the filter with the id if
// it is not empty, and returns the id of the saved filter.
func (s *savedFilterTest) save(u *models.User, id string, name string, personal bool) (string, []error) {
	s.t.Helper()

	idArg := ""
	if id != "" {
		idArg = fmt.Sprintf("id: %q,", id)
	}

	data, errs := s.run(u, `mutation { saveFilter(input: {%s mode: SCENES, name: %q, personal: %t}) { id } }`, idArg, name, personal)
	if len(errs) > 0 {
		return "", errs
	}

	return data["saveFilter"].(map[string]interface{})["id"].(string), nil
}

func (s *savedFilterTest) destroy(u *models.User, id string) []error {
	s.t.Helper()

	_, errs := s.run(u, `mutation { destroySavedFilter(input: {id: %q}) }`, id)
	return errs
}

func (s *savedFilterTest) share(u *models.User, id string) (string, []error) {
	s.t.Helper()

	data, errs := s.run(u, `mutation { shareSavedFilter(input: {id: %q}) { id } }`, id)
	if len(errs) > 0 {
		return "", errs
	}

	return data["shareSavedFilter"].(map[string]interface{})["id"].(string), nil
}

// names returns the names of the scene filters that the user sees, in the
// order of findSavedFilters, with "*" after the personal ones.
func (s *savedFilterTest) names(u *models.User) []string {
	s.t.Helper()

	data, errs := s.run(u, `{ findSavedFilters(mode: SCENES) { name personal } }`)
	require.Empty(s.t, errs)

	var ret []string
	for _, f := range data["findSavedFilters"].([]interface{}) {
		m := f.(map[string]interface{})
		name := m["name"].(string)
		if m["personal"].(bool) {
			name += "*"
		}
		ret = append(ret, name)
	}

	return ret
}

func isPersonalID(t *testing.T, id string) bool {
	t.Helper()

	n, err := strconv.Atoi(id)
	require.NoError(t, err)
	return models.IsPersonalSavedFilterID(n)
}

// TestPersonalSavedFilterRights checks that every signed-in user, whatever
// their role, saves, overwrites, renames and deletes their own personal
// filters.
func TestPersonalSavedFilterRights(t *testing.T) {
	for _, roleID := range []string{models.RoleIDViewer, models.RoleIDEditor, models.RoleIDAdmin} {
		t.Run(roleID, func(t *testing.T) {
			s := newSavedFilterTest(t)
			u := savedFilterUser("user", roleID)

			id, errs := s.save(u, "", "Mine", true)
			require.Empty(t, errs, "create")
			assert.True(t, isPersonalID(t, id), "personal id %s", id)

			_, errs = s.save(u, "", "Mine", true)
			if assert.Len(t, errs, 1, "name taken") {
				assert.Contains(t, errs[0].Error(), `a saved filter named "Mine" already exists`)
			}

			// personal is ignored when overwriting
			_, errs = s.save(u, id, "Renamed", false)
			require.Empty(t, errs, "overwrite and rename")
			assert.Equal(t, []string{"Renamed*"}, s.names(u))

			require.Empty(t, s.destroy(u, id), "delete")
			assert.Empty(t, s.names(u))
		})
	}
}

// TestSharedSavedFilterRights checks that the shared filters keep their
// permissions: SAVE_FILTERS to create, overwrite and rename them, and
// DELETE_SAVED_FILTERS to delete them.
func TestSharedSavedFilterRights(t *testing.T) {
	admin := savedFilterUser("admin", models.RoleIDAdmin)
	editor := savedFilterUser("editor", models.RoleIDEditor)
	viewer := savedFilterUser("viewer", models.RoleIDViewer)

	s := newSavedFilterTest(t)

	_, errs := s.save(viewer, "", "Viewer's", false)
	assertDenied(t, errs, requirement{permission: models.PermissionSaveFilters}, "viewer creates")

	id, errs := s.save(editor, "", "Shared", false)
	require.Empty(t, errs, "editor creates")
	assert.False(t, isPersonalID(t, id), "shared id %s", id)

	_, errs = s.save(viewer, id, "Viewer's", false)
	assertDenied(t, errs, requirement{permission: models.PermissionSaveFilters}, "viewer overwrites")

	_, errs = s.save(editor, id, "Renamed", false)
	require.Empty(t, errs, "editor overwrites")

	_, errs = s.save(editor, "", "Renamed", false)
	if assert.Len(t, errs, 1, "name taken") {
		assert.Contains(t, errs[0].Error(), `a saved filter named "Renamed" already exists`)
	}

	assertDenied(t, s.destroy(editor, id), requirement{permission: models.PermissionDeleteSavedFilters}, "editor deletes")
	assertDenied(t, s.destroy(viewer, id), requirement{permission: models.PermissionDeleteSavedFilters}, "viewer deletes")
	assert.Equal(t, []string{"Renamed"}, s.names(viewer), "everyone sees it")

	require.Empty(t, s.destroy(admin, id), "admin deletes")
	assert.Empty(t, s.names(viewer))
}

// TestPersonalSavedFilterIsolation checks that nobody else, not even an
// admin, sees or changes the personal filter of a user.
func TestPersonalSavedFilterIsolation(t *testing.T) {
	alice := savedFilterUser("alice", models.RoleIDViewer)
	admin := savedFilterUser("admin", models.RoleIDAdmin)

	s := newSavedFilterTest(t)
	id, errs := s.save(alice, "", "Alice's", true)
	require.Empty(t, errs)

	assert.Empty(t, s.names(admin), "listed")

	data, errs := s.run(admin, `{ findSavedFilter(id: %q) { id } }`, id)
	require.Empty(t, errs)
	assert.Nil(t, data["findSavedFilter"], "found by id")

	data, errs = s.run(alice, `{ findSavedFilter(id: %q) { id personal } }`, id)
	require.Empty(t, errs)
	assert.Equal(t, map[string]interface{}{"id": id, "personal": true}, data["findSavedFilter"], "found by its owner")

	_, errs = s.save(admin, id, "Admin's", false)
	if assert.Len(t, errs, 1, "overwritten") {
		assert.Contains(t, errs[0].Error(), fmt.Sprintf("saved filter %s not found", id))
	}
	assert.Len(t, s.destroy(admin, id), 1, "deleted")
	_, errs = s.share(admin, id)
	assert.Len(t, errs, 1, "shared")

	assert.Equal(t, []string{"Alice's*"}, s.names(alice), "unchanged")

	_, errs = s.save(admin, "", "Alice's", true)
	assert.Empty(t, errs, "the name is free for another user")
}

// TestShareSavedFilter checks that sharing moves a personal filter to the
// shared filters, and that it needs SAVE_FILTERS and a free name.
func TestShareSavedFilter(t *testing.T) {
	editor := savedFilterUser("editor", models.RoleIDEditor)
	viewer := savedFilterUser("viewer", models.RoleIDViewer)

	s := newSavedFilterTest(t)

	viewerID, errs := s.save(viewer, "", "Viewer's", true)
	require.Empty(t, errs)
	_, errs = s.share(viewer, viewerID)
	assertDenied(t, errs, requirement{permission: models.PermissionSaveFilters}, "viewer shares")

	id, errs := s.save(editor, "", "Favourites", true)
	require.Empty(t, errs)
	sharedID, errs := s.share(editor, id)
	require.Empty(t, errs, "editor shares")
	assert.False(t, isPersonalID(t, sharedID), "shared id %s", sharedID)
	assert.Equal(t, []string{"Favourites", "Viewer's*"}, s.names(viewer), "shared with everyone")
	assert.Equal(t, []string{"Favourites"}, s.names(editor), "no longer personal")

	_, errs = s.share(editor, sharedID)
	assert.Len(t, errs, 1, "a shared filter")

	// a taken name keeps the personal filter
	id, errs = s.save(editor, "", "Favourites", true)
	require.Empty(t, errs, "personal filter with the name of a shared one")
	_, errs = s.share(editor, id)
	if assert.Len(t, errs, 1, "name taken") {
		assert.Contains(t, errs[0].Error(), `a shared filter named "Favourites" already exists`)
	}
	assert.Equal(t, []string{"Favourites*", "Favourites"}, s.names(editor), "personal first on equal names")
}

// TestFindSavedFiltersMerged checks that the personal filters are listed with
// the shared ones by name, capitals first, as the database sorts them.
func TestFindSavedFiltersMerged(t *testing.T) {
	editor := savedFilterUser("editor", models.RoleIDEditor)

	s := newSavedFilterTest(t)
	for _, f := range []struct {
		name     string
		personal bool
	}{{"b", false}, {"B", true}, {"a", true}, {"A", false}} {
		_, errs := s.save(editor, "", f.name, f.personal)
		require.Empty(t, errs, f.name)
	}

	assert.Equal(t, []string{"A", "B*", "a*", "b"}, s.names(editor), "by mode")

	data, errs := s.run(editor, `{ findSavedFilters { name } }`)
	require.Empty(t, errs)
	var all []string
	for _, f := range data["findSavedFilters"].([]interface{}) {
		all = append(all, f.(map[string]interface{})["name"].(string))
	}
	assert.Equal(t, []string{"A", "B", "a", "b"}, all, "every mode")
}

// TestPersonalSavedFilterWithoutUser checks that without authentication, or
// without a user name, there are no personal filters, and the shared filters
// work as before.
func TestPersonalSavedFilterWithoutUser(t *testing.T) {
	s := newSavedFilterTest(t)

	_, errs := s.save(nil, "", "Personal", true)
	if assert.Len(t, errs, 1, "personal") {
		assert.Contains(t, errs[0].Error(), "personal saved filters need a signed-in user")
	}

	id, errs := s.save(nil, "", "Shared", false)
	require.Empty(t, errs, "shared")
	assert.Equal(t, []string{"Shared"}, s.names(nil))

	// a plugin call that no user triggered has an Admin without a name
	plugin := &models.User{Role: models.RoleIDAdmin}
	_, errs = s.save(plugin, "", "Plugin's", true)
	if assert.Len(t, errs, 1, "personal for a plugin") {
		assert.Contains(t, errs[0].Error(), "personal saved filters need a signed-in user")
	}
	assert.Equal(t, []string{"Shared"}, s.names(plugin), "a plugin sees the shared filters")

	require.Empty(t, s.destroy(nil, id))

	// neither finds nor deletes the personal filters of a user
	alice := savedFilterUser("alice", models.RoleIDViewer)
	personalID, errs := s.save(alice, "", "Alice's", true)
	require.Empty(t, errs)
	for _, u := range []*models.User{nil, plugin} {
		data, errs := s.run(u, `{ findSavedFilter(id: %q) { id } }`, personalID)
		require.Empty(t, errs)
		assert.Nil(t, data["findSavedFilter"], "found")
		assert.Len(t, s.destroy(u, personalID), 1, "deleted")
	}
	assert.Equal(t, []string{"Alice's*"}, s.names(alice), "unchanged")
}

// TestFindSavedFiltersGroupsIncludeMovies checks that the group filters
// include the filters saved with the movies mode, personal and shared alike.
func TestFindSavedFiltersGroupsIncludeMovies(t *testing.T) {
	s := newSavedFilterTest(t)
	u := savedFilterUser("user", models.RoleIDAdmin)

	for _, f := range []struct {
		mode     string
		name     string
		personal bool
	}{
		{"MOVIES", "Personal movies", true},
		{"GROUPS", "Personal groups", true},
		{"MOVIES", "Shared movies", false},
	} {
		_, errs := s.run(u, `mutation { saveFilter(input: {mode: %s, name: %q, personal: %t}) { id } }`, f.mode, f.name, f.personal)
		require.Empty(t, errs, f.name)
	}

	data, errs := s.run(u, `{ findSavedFilters(mode: GROUPS) { name } }`)
	require.Empty(t, errs)

	var names []string
	for _, f := range data["findSavedFilters"].([]interface{}) {
		names = append(names, f.(map[string]interface{})["name"].(string))
	}
	assert.ElementsMatch(t, []string{"Personal movies", "Personal groups", "Shared movies"}, names)
}

// createListedUsers adds the users to the users list, with personal settings.
// The first must be an admin.
func createListedUsers(t *testing.T, users ...*models.User) {
	t.Helper()

	for _, u := range users {
		require.NoError(t, config.GetInstance().UserStore.CreateUser(context.Background(), *u, "password"))
	}
}

// setFrontPage sets the personal front page of the user to rows of the saved
// filters with the ids: json.Number, as GraphQL variables decode the ids that
// the UI sends, or text, as the config file stores them.
func setFrontPage(t *testing.T, username string, ids ...interface{}) {
	t.Helper()

	rows := make([]interface{}, len(ids))
	for i, id := range ids {
		rows[i] = map[string]interface{}{"__typename": "SavedFilter", "savedFilterId": id}
	}

	require.NoError(t, config.GetInstance().UserStore.UpdatePersonalSettings(username, func(_, ui map[string]interface{}) error {
		ui["frontPageContent"] = rows
		return nil
	}))
}

// frontPageIDs returns the ids of the saved filters of the front page rows of
// the UI configuration.
func frontPageIDs(ui map[string]interface{}) []string {
	var ret []string
	rows, _ := ui["frontPageContent"].([]interface{})
	for _, row := range rows {
		ret = append(ret, fmt.Sprint(row.(map[string]interface{})["savedFilterId"]))
	}

	return ret
}

// TestPublishFrontPageWithPersonalFilters checks that a front page with rows
// of personal filters, which the other users cannot see, is not published.
func TestPublishFrontPageWithPersonalFilters(t *testing.T) {
	s := newSavedFilterTest(t)
	admin := savedFilterUser("admin", models.RoleIDAdmin)
	createListedUsers(t, admin)
	cfg := config.GetInstance()
	const publish = `mutation { publishPersonalSettings(settings: [{section: UI, key: "frontPageContent"}]) }`

	id, errs := s.save(admin, "", "Admin picks", true)
	require.Empty(t, errs)
	setFrontPage(t, "admin", json.Number(id))

	_, errs = s.run(admin, publish)
	if assert.Len(t, errs, 1, "personal filter row") {
		assert.Contains(t, errs[0].Error(), `"Admin picks"`)
	}
	assert.Nil(t, cfg.GetUIConfiguration()["frontPageContent"], "nothing is published")

	sharedID, errs := s.share(admin, id)
	require.Empty(t, errs)
	setFrontPage(t, "admin", json.Number(sharedID))

	_, errs = s.run(admin, publish)
	require.Empty(t, errs, "shared filter row")
	assert.Equal(t, []string{sharedID}, frontPageIDs(cfg.GetUIConfiguration()))
}

// TestShareSavedFilterKeepsFrontPageRow checks that the row of a shared filter
// stays on the personal front page of the user who shares it.
func TestShareSavedFilterKeepsFrontPageRow(t *testing.T) {
	for _, c := range []struct {
		name string
		id   func(string) interface{}
	}{
		{"as saved", func(id string) interface{} { return json.Number(id) }},
		{"as read from the config file", func(id string) interface{} { return id }},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := newSavedFilterTest(t)
			admin := savedFilterUser("admin", models.RoleIDAdmin)
			editor := savedFilterUser("editor", models.RoleIDEditor)
			createListedUsers(t, admin, editor)

			id, errs := s.save(editor, "", "Favourites", true)
			require.Empty(t, errs)
			other, errs := s.save(editor, "", "Other", true)
			require.Empty(t, errs)
			setFrontPage(t, "editor", c.id(id), c.id(other))

			sharedID, errs := s.share(editor, id)
			require.Empty(t, errs)

			_, ui, err := config.GetInstance().UserStore.PersonalSettings("editor")
			require.NoError(t, err)
			assert.Equal(t, []string{sharedID, other}, frontPageIDs(ui))
		})
	}
}
