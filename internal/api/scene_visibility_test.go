//go:build integration
// +build integration

package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/go-chi/chi/v5"
	"github.com/stashapp/stash/internal/api/loaders"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/plugin"
	"github.com/stashapp/stash/pkg/scene"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sceneVisibilities = []models.SceneVisibility{models.SceneVisibilityEveryone, models.SceneVisibilityRestricted, models.SceneVisibilityPrivate}

// sceneVisibilityTest runs GraphQL operations against a new database that has
// a scene of each visibility.
type sceneVisibilityTest struct {
	t    *testing.T
	cfg  *config.Config
	db   *sqlite.Database
	repo models.Repository
	exec *executor.Executor

	// scene ID and the oshash of its file, by the visibility of the scene
	sceneIDs map[models.SceneVisibility]int
	hashes   map[models.SceneVisibility]string
	tagID    int
}

func newSceneVisibilityTest(t *testing.T) *sceneVisibilityTest {
	t.Helper()

	cfg := config.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))

	db := sqlite.NewDatabase()
	db.SetBlobStoreOptions(sqlite.BlobStoreOptions{UseDatabase: true})
	require.NoError(t, db.Open(filepath.Join(t.TempDir(), "stash-go.sqlite")))
	t.Cleanup(func() { _ = db.Close() })
	db.SetDefaultSceneVisibility(cfg.GetDefaultSceneVisibility)

	repo := db.Repository()
	r := &Resolver{
		repository: repo,
		sceneService: &scene.Service{
			File:             repo.File,
			Repository:       repo.Scene,
			MarkerRepository: repo.SceneMarker,
			PluginCache:      plugin.NewCache(cfg),
		},
		hookExecutor: &mockHookExecutor{},
	}

	exec := executor.New(NewExecutableSchema(Config{
		Resolvers: r,
		Directives: DirectiveRoot{
			HasPermission: HasPermissionDirective,
			AdminOnly:     AdminOnlyDirective,
			SignedIn:      SignedInDirective,
			IsUserOwner:   IsUserOwnerDirective,
		},
	}))
	exec.AroundRootFields(mutationRequirementMiddleware)

	v := &sceneVisibilityTest{
		t:        t,
		cfg:      cfg,
		db:       db,
		repo:     repo,
		exec:     exec,
		sceneIDs: make(map[models.SceneVisibility]int),
		hashes:   make(map[models.SceneVisibility]string),
	}

	require.NoError(t, txn.WithTxn(context.Background(), db, func(ctx context.Context) error {
		folder := models.Folder{Path: "videos", DirEntry: models.DirEntry{ModTime: time.Now()}}
		if err := db.Folder.Create(ctx, &folder); err != nil {
			return err
		}

		for _, visibility := range sceneVisibilities {
			hash := "hash" + visibility.String()
			f := &models.VideoFile{
				BaseFile: &models.BaseFile{
					Path:           filepath.Join("videos", hash+".mp4"),
					Basename:       hash + ".mp4",
					ParentFolderID: folder.ID,
					Fingerprints:   []models.Fingerprint{{Type: models.FingerprintTypeOshash, Fingerprint: hash}},
				},
			}
			if err := db.File.Create(ctx, f); err != nil {
				return err
			}

			s := models.NewScene()
			s.Title = visibility.String()
			if err := db.Scene.Create(ctx, &s, []models.FileID{f.ID}); err != nil {
				return err
			}

			if _, err := db.Scene.UpdatePartial(ctx, s.ID, models.ScenePartial{Visibility: &visibility}); err != nil {
				return err
			}

			v.sceneIDs[visibility] = s.ID
			v.hashes[visibility] = hash
		}

		tag := models.NewTag()
		tag.Name = "tag"
		if err := db.Tag.Create(ctx, &models.CreateTagInput{Tag: &tag}); err != nil {
			return err
		}
		v.tagID = tag.ID

		return nil
	}))

	return v
}

// roleUser returns a user with the role with the ID - Admin or a default role
// - or no user if the ID is empty.
func roleUser(roleID string) *models.User {
	if roleID == "" {
		return nil
	}

	u := models.User{Username: "user", Role: roleID}
	for _, r := range models.DefaultRoles() {
		if r.ID == roleID {
			u.Permissions = r.Permissions
		}
	}

	return &u
}

// userContext returns a context of a user with the role with the ID - Admin
// or a default role - or of no user if the ID is empty.
func userContext(ctx context.Context, roleID string) context.Context {
	if u := roleUser(roleID); u != nil {
		return models.SetCurrentUser(ctx, *u)
	}

	return ctx
}

// run executes the operation as a user with the role, and returns the
// response data and errors.
func (v *sceneVisibilityTest) run(roleID string, query string, args ...interface{}) (map[string]interface{}, []error) {
	v.t.Helper()

	return v.runAs(roleUser(roleID), query, args...)
}

// runAs executes the operation as the user, or without a user if u is nil, and
// returns the response data and errors.
func (v *sceneVisibilityTest) runAs(u *models.User, query string, args ...interface{}) (map[string]interface{}, []error) {
	v.t.Helper()

	// the request context of the server: the user and the dataloaders
	var ctx context.Context
	r := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	if u != nil {
		r = r.WithContext(models.SetCurrentUser(r.Context(), *u))
	}
	loaders.Middleware{Repository: v.repo}.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx = r.Context()
	})).ServeHTTP(httptest.NewRecorder(), r)
	ctx = graphql.StartOperationTrace(ctx)

	opCtx, errs := v.exec.CreateOperationContext(ctx, &graphql.RawParams{Query: fmt.Sprintf(query, args...)})
	require.Empty(v.t, errs)

	handler, ctx := v.exec.DispatchOperation(ctx, opCtx)
	resp := handler(ctx)
	require.NotNil(v.t, resp)

	var data map[string]interface{}
	if len(resp.Data) > 0 {
		require.NoError(v.t, json.Unmarshal(resp.Data, &data))
	}

	var retErrs []error
	for _, err := range resp.Errors {
		retErrs = append(retErrs, err)
	}

	return data, retErrs
}

// mustRun executes the operation and fails if it returns errors.
func (v *sceneVisibilityTest) mustRun(roleID string, query string, args ...interface{}) map[string]interface{} {
	v.t.Helper()

	data, errs := v.run(roleID, query, args...)
	require.Empty(v.t, errs)
	return data
}

func (v *sceneVisibilityTest) visibility(id int) models.SceneVisibility {
	v.t.Helper()

	var ret models.SceneVisibility
	require.NoError(v.t, txn.WithReadTxn(context.Background(), v.db, func(ctx context.Context) error {
		var err error
		ret, err = v.db.Scene.GetVisibility(ctx, id)
		return err
	}))
	return ret
}

func (v *sceneVisibilityTest) sceneFileIDs(id int) []models.FileID {
	v.t.Helper()

	var ret [][]models.FileID
	require.NoError(v.t, txn.WithReadTxn(context.Background(), v.db, func(ctx context.Context) error {
		var err error
		ret, err = v.db.Scene.GetManyFileIDs(ctx, []int{id})
		return err
	}))
	return ret[0]
}

func assertDenied(t *testing.T, errs []error, req requirement, msg string) {
	t.Helper()

	if assert.Len(t, errs, 1, msg) {
		assertPermissionDenied(t, errs[0], req)
	}
}

func TestSceneVisibilityFindScenes(t *testing.T) {
	v := newSceneVisibilityTest(t)

	tests := []struct {
		roleID  string
		visible []models.SceneVisibility
	}{
		{"", sceneVisibilities},
		{models.RoleIDViewer, []models.SceneVisibility{models.SceneVisibilityEveryone}},
		{models.RoleIDEditor, []models.SceneVisibility{models.SceneVisibilityEveryone, models.SceneVisibilityRestricted}},
		{models.RoleIDAdmin, sceneVisibilities},
	}

	for _, tt := range tests {
		t.Run("user "+tt.roleID, func(t *testing.T) {
			data := v.mustRun(tt.roleID, `query { findScenes { count scenes { id visibility } } }`)
			result := data["findScenes"].(map[string]interface{})
			assert.EqualValues(t, len(tt.visible), result["count"])

			var got []string
			for _, s := range result["scenes"].([]interface{}) {
				got = append(got, s.(map[string]interface{})["visibility"].(string))
			}
			var want []string
			for _, r := range tt.visible {
				want = append(want, r.String())
			}
			assert.ElementsMatch(t, want, got)

			for _, visibility := range sceneVisibilities {
				data := v.mustRun(tt.roleID, `query { findScene(id: "%d") { id } }`, v.sceneIDs[visibility])
				hidden := !slices.Contains(tt.visible, visibility)
				assert.Equal(t, hidden, data["findScene"] == nil, "findScene of %s scene", visibility)
			}
		})
	}
}

func TestSceneVisibilityChangePermission(t *testing.T) {
	v := newSceneVisibilityTest(t)
	publicID := v.sceneIDs[models.SceneVisibilityEveryone]
	changeVisibility := requirement{permission: models.PermissionChangeSceneVisibility}

	// editors can update scenes, but not their visibility
	editor := models.RoleIDEditor
	_, errs := v.run(editor, `mutation { sceneUpdate(input: {id: "%d", visibility: PRIVATE}) { id } }`, publicID)
	assertDenied(t, errs, changeVisibility, "sceneUpdate")

	_, errs = v.run(editor, `mutation { bulkSceneUpdate(input: {ids: ["%d"], visibility: PRIVATE}) { id } }`, publicID)
	assertDenied(t, errs, changeVisibility, "bulkSceneUpdate")

	_, errs = v.run(editor, `mutation { scenesUpdate(input: [{id: "%d", visibility: PRIVATE}]) { id } }`, publicID)
	assertDenied(t, errs, changeVisibility, "scenesUpdate")

	assert.Equal(t, models.SceneVisibilityEveryone, v.visibility(publicID), "visibility after denied changes")

	// anyone can filter by visibility, which only matches the scenes they can see
	data := v.mustRun(editor, `query { findScenes(scene_filter: {visibility: {value: [PRIVATE], modifier: INCLUDES}}) { count } }`)
	assert.EqualValues(t, 0, data["findScenes"].(map[string]interface{})["count"], "editor filtering private scenes")

	data = v.mustRun(editor, `query { findScenes(scene_filter: {visibility: {value: [RESTRICTED], modifier: INCLUDES}}) { count } }`)
	assert.EqualValues(t, 1, data["findScenes"].(map[string]interface{})["count"], "editor filtering restricted scenes")

	// editing other fields is not restricted
	v.mustRun(editor, `mutation { sceneUpdate(input: {id: "%d", title: "changed"}) { id } }`, publicID)

	data = v.mustRun(models.RoleIDAdmin, `query { findScenes(scene_filter: {visibility: {value: [PRIVATE], modifier: INCLUDES}}) { count } }`)
	assert.EqualValues(t, 1, data["findScenes"].(map[string]interface{})["count"])

	data = v.mustRun(models.RoleIDAdmin, `mutation { sceneUpdate(input: {id: "%d", visibility: RESTRICTED}) { visibility } }`, publicID)
	assert.Equal(t, "RESTRICTED", data["sceneUpdate"].(map[string]interface{})["visibility"])
	assert.Equal(t, models.SceneVisibilityRestricted, v.visibility(publicID))

	v.mustRun(models.RoleIDAdmin, `mutation { bulkSceneUpdate(input: {ids: ["%d"], visibility: PRIVATE}) { id } }`, publicID)
	assert.Equal(t, models.SceneVisibilityPrivate, v.visibility(publicID))

	v.mustRun(models.RoleIDAdmin, `mutation { scenesUpdate(input: [{id: "%d", visibility: EVERYONE}]) { id } }`, publicID)
	assert.Equal(t, models.SceneVisibilityEveryone, v.visibility(publicID))
}

// TestSceneVisibilityMoveFiles checks that merging scenes, and moving a file,
// between scenes of different visibility, which changes who sees the files,
// requires the permission to change the visibility.
func TestSceneVisibilityMoveFiles(t *testing.T) {
	v := newSceneVisibilityTest(t)
	everyone := v.sceneIDs[models.SceneVisibilityEveryone]
	restricted := v.sceneIDs[models.SceneVisibilityRestricted]
	private := v.sceneIDs[models.SceneVisibilityPrivate]
	changeVisibility := requirement{permission: models.PermissionChangeSceneVisibility}

	// a second file of the restricted scene, a file without a scene, and
	// another restricted scene
	var fileID, looseID models.FileID
	var otherRestricted int
	require.NoError(t, txn.WithTxn(context.Background(), v.db, func(ctx context.Context) error {
		folder := models.Folder{Path: "extra", DirEntry: models.DirEntry{ModTime: time.Now()}}
		if err := v.db.Folder.Create(ctx, &folder); err != nil {
			return err
		}

		loose := &models.VideoFile{BaseFile: &models.BaseFile{
			Path:           filepath.Join("extra", "loose.mp4"),
			Basename:       "loose.mp4",
			ParentFolderID: folder.ID,
		}}
		if err := v.db.File.Create(ctx, loose); err != nil {
			return err
		}
		looseID = loose.ID

		f := &models.VideoFile{BaseFile: &models.BaseFile{
			Path:           filepath.Join("extra", "second.mp4"),
			Basename:       "second.mp4",
			ParentFolderID: folder.ID,
		}}
		if err := v.db.File.Create(ctx, f); err != nil {
			return err
		}
		fileID = f.ID

		if err := v.db.Scene.AssignFiles(ctx, restricted, []models.FileID{f.ID}); err != nil {
			return err
		}

		// with a primary file, so that the moved file is not made primary
		primary := &models.VideoFile{BaseFile: &models.BaseFile{
			Path:           filepath.Join("extra", "primary.mp4"),
			Basename:       "primary.mp4",
			ParentFolderID: folder.ID,
		}}
		if err := v.db.File.Create(ctx, primary); err != nil {
			return err
		}

		s := models.NewScene()
		if err := v.db.Scene.Create(ctx, &s, []models.FileID{primary.ID}); err != nil {
			return err
		}
		otherRestricted = s.ID

		visibility := models.SceneVisibilityRestricted
		_, err := v.db.Scene.UpdatePartial(ctx, s.ID, models.ScenePartial{Visibility: &visibility})
		return err
	}))

	mover := &models.User{Username: "mover", Role: "custom", Permissions: models.Permissions{
		models.PermissionScenesMerge,
		models.PermissionManageFiles,
		models.PermissionViewRestrictedScenes,
		models.PermissionViewPrivateScenes,
	}}
	const merge = `mutation { sceneMerge(input: {source: ["%d"], destination: "%d"}) { id } }`
	const assign = `mutation { sceneAssignFile(input: {scene_id: "%d", file_id: "%d"}) }`

	_, errs := v.runAs(mover, merge, private, everyone)
	assertDenied(t, errs, changeVisibility, "merge into another visibility")

	_, errs = v.runAs(mover, assign, everyone, fileID)
	assertDenied(t, errs, changeVisibility, "file to another visibility")

	_, errs = v.runAs(mover, assign, otherRestricted, fileID)
	assert.Empty(t, errs, "file to the same visibility")

	mover.Permissions = append(mover.Permissions, models.PermissionChangeSceneVisibility)
	_, errs = v.runAs(mover, assign, everyone, fileID)
	assert.Empty(t, errs, "file to another visibility, with the permission")

	// hidden scenes are not found, with the permission too
	hider := &models.User{Username: "hider", Role: "custom", Permissions: models.Permissions{
		models.PermissionScenesMerge,
		models.PermissionManageFiles,
		models.PermissionViewRestrictedScenes,
	}}
	for _, permission := range []bool{false, true} {
		if permission {
			hider.Permissions = append(hider.Permissions, models.PermissionChangeSceneVisibility)
		}

		for _, c := range []struct {
			name     string
			query    string
			from, to int
		}{
			{"file to a hidden scene", assign, private, int(fileID)},
			{"file without a scene to a hidden scene", assign, private, int(looseID)},
			{"merge into a hidden scene", merge, otherRestricted, private},
			{"merge of a hidden scene", merge, private, otherRestricted},
		} {
			_, errs := v.runAs(hider, c.query, c.from, c.to)
			if assert.Len(t, errs, 1, "%s, with the permission: %v", c.name, permission) {
				assert.Regexp(t, notFoundRE, errs[0].Error(), "%s, with the permission: %v", c.name, permission)
			}
		}
	}

	assert.NotContains(t, v.sceneFileIDs(private), fileID, "files of the hidden scene")
	assert.NotContains(t, v.sceneFileIDs(private), looseID, "files of the hidden scene")
}

var notFoundRE = regexp.MustCompile("not found|does not exist")

// TestSceneVisibilityHiddenFiles checks that the files of the scenes hidden
// from a user who manages files are not found.
func TestSceneVisibilityHiddenFiles(t *testing.T) {
	v := newSceneVisibilityTest(t)
	hiddenID := v.sceneIDs[models.SceneVisibilityPrivate]
	visibleID := v.sceneIDs[models.SceneVisibilityEveryone]
	const files = `query { findScene(id: "%d") { files { id fingerprints { type } } } }`

	data := v.mustRun(models.RoleIDAdmin, files, hiddenID)
	file := data["findScene"].(map[string]interface{})["files"].([]interface{})[0].(map[string]interface{})
	fileID := file["id"].(string)

	filer := &models.User{Username: "filer", Role: "custom", Permissions: models.Permissions{
		models.PermissionManageFiles,
		models.PermissionViewRestrictedScenes,
	}}

	_, errs := v.runAs(filer, `mutation { fileSetFingerprints(input: {id: "%s", fingerprints: [{type: "phash", value: "1"}]}) }`, fileID)
	if assert.Len(t, errs, 1, "fileSetFingerprints") {
		assert.Regexp(t, notFoundRE, errs[0].Error())
	}

	_, errs = v.runAs(filer, `mutation { sceneAssignFile(input: {scene_id: "%d", file_id: "%s"}) }`, visibleID, fileID)
	if assert.Len(t, errs, 1, "sceneAssignFile") {
		assert.Regexp(t, notFoundRE, errs[0].Error())
	}

	data = v.mustRun(models.RoleIDAdmin, files, hiddenID)
	file = data["findScene"].(map[string]interface{})["files"].([]interface{})[0].(map[string]interface{})
	assert.Equal(t, []interface{}{map[string]interface{}{"type": "oshash"}}, file["fingerprints"], "unchanged")
}

func TestSceneVisibilityHiddenMutations(t *testing.T) {
	v := newSceneVisibilityTest(t)

	mutations := []struct {
		name  string
		query string
		// false if the mutation cannot run in the test when the scene is visible
		runsVisible bool
	}{
		{"sceneUpdate", `mutation { sceneUpdate(input: {id: "%d", title: "changed"}) { id } }`, true},
		{"bulkSceneUpdate", `mutation { bulkSceneUpdate(input: {ids: ["%d"], title: "changed"}) { id } }`, true},
		{"scenesUpdate", `mutation { scenesUpdate(input: [{id: "%d", title: "changed"}]) { id } }`, true},
		{"sceneSaveActivity", `mutation { sceneSaveActivity(id: "%d", resume_time: 1) }`, true},
		{"sceneResetActivity", `mutation { sceneResetActivity(id: "%d", reset_resume: true) }`, true},
		{"sceneAddO", `mutation { sceneAddO(id: "%d") { count } }`, true},
		{"sceneDeleteO", `mutation { sceneDeleteO(id: "%d") { count } }`, true},
		{"sceneResetO", `mutation { sceneResetO(id: "%d") }`, true},
		{"sceneAddPlay", `mutation { sceneAddPlay(id: "%d") { count } }`, true},
		{"sceneDeletePlay", `mutation { sceneDeletePlay(id: "%d") { count } }`, true},
		{"sceneResetPlayCount", `mutation { sceneResetPlayCount(id: "%d") }`, true},
		{"sceneMarkerCreate", `mutation { sceneMarkerCreate(input: {title: "marker", seconds: 1, scene_id: "%d", primary_tag_id: "` + fmt.Sprint(v.tagID) + `"}) { id } }`, true},
		// starts a job of the manager, which is not set up in the test
		{"sceneGenerateScreenshot", `mutation { sceneGenerateScreenshot(id: "%d") }`, false},
	}

	hiddenID := v.sceneIDs[models.SceneVisibilityPrivate]
	visibleID := v.sceneIDs[models.SceneVisibilityRestricted]

	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			_, errs := v.run(models.RoleIDEditor, m.query, hiddenID)
			if assert.Len(t, errs, 1, "hidden scene") {
				assert.Regexp(t, notFoundRE, errs[0].Error())
			}

			if m.runsVisible {
				_, errs = v.run(models.RoleIDEditor, m.query, visibleID)
				assert.Empty(t, errs, "visible scene")
			}
		})
	}

	// the hidden scene is unchanged
	data := v.mustRun(models.RoleIDAdmin, `query { findScene(id: "%d") { title o_counter play_count resume_time scene_markers { id } } }`, hiddenID)
	s := data["findScene"].(map[string]interface{})
	assert.Equal(t, "PRIVATE", s["title"])
	assert.EqualValues(t, 0, s["o_counter"])
	assert.EqualValues(t, 0, s["play_count"])
	assert.EqualValues(t, 0, s["resume_time"])
	assert.Empty(t, s["scene_markers"])
}

func TestSceneVisibilityMediaRoutes(t *testing.T) {
	v := newSceneVisibilityTest(t)

	rs := sceneRoutes{
		routes:      routes{txnManager: v.db},
		sceneFinder: v.db.Scene,
		fileGetter:  v.db.File,
	}

	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// the scene checks, in front of a handler that always succeeds
	checked := chi.NewRouter()
	checked.With(rs.SceneCtx).Get("/{sceneId}/screenshot", ok)
	checked.With(rs.SceneHashCtx).Get("/{sceneHash}_thumbs.vtt", ok)

	serve := func(router http.Handler, roleID string, target string) int {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		r = r.WithContext(userContext(r.Context(), roleID))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w.Code
	}

	viewerHidden := v.sceneIDs[models.SceneVisibilityRestricted]
	viewerHash := v.hashes[models.SceneVisibilityRestricted]

	// the routes of the server are checked
	routes := rs.Routes()
	for _, target := range []string{
		fmt.Sprintf("/%d/screenshot", viewerHidden),
		fmt.Sprintf("/%d/stream", viewerHidden),
		fmt.Sprintf("/%d/vtt/chapter", viewerHidden),
		fmt.Sprintf("/%s_thumbs.vtt", viewerHash),
		fmt.Sprintf("/%s_sprite.jpg", viewerHash),
	} {
		assert.Equal(t, http.StatusNotFound, serve(routes, models.RoleIDViewer, target), "viewer %s", target)
	}

	tests := []struct {
		roleID     string
		visibility models.SceneVisibility
		want       int
	}{
		{models.RoleIDViewer, models.SceneVisibilityEveryone, http.StatusOK},
		{models.RoleIDViewer, models.SceneVisibilityRestricted, http.StatusNotFound},
		{models.RoleIDEditor, models.SceneVisibilityRestricted, http.StatusOK},
		{models.RoleIDEditor, models.SceneVisibilityPrivate, http.StatusNotFound},
		{models.RoleIDAdmin, models.SceneVisibilityPrivate, http.StatusOK},
		{"", models.SceneVisibilityPrivate, http.StatusOK},
	}

	for _, tt := range tests {
		id := v.sceneIDs[tt.visibility]
		assert.Equal(t, tt.want, serve(checked, tt.roleID, fmt.Sprintf("/%d/screenshot", id)), "user %q, %s scene by id", tt.roleID, tt.visibility)

		hash := v.hashes[tt.visibility]
		assert.Equal(t, tt.want, serve(checked, tt.roleID, fmt.Sprintf("/%s_thumbs.vtt", hash)), "user %q, %s scene by hash", tt.roleID, tt.visibility)
	}

	assert.Equal(t, http.StatusNotFound, serve(checked, models.RoleIDAdmin, "/unknown_thumbs.vtt"), "unknown hash")
}

func TestSceneVisibilityDefault(t *testing.T) {
	v := newSceneVisibilityTest(t)

	assert.Equal(t, models.SceneVisibilityEveryone, makeConfigGeneralResult().DefaultSceneVisibility, "default")

	v.cfg.SetString(config.DefaultSceneVisibility, models.SceneVisibilityPrivate.String())
	assert.Equal(t, models.SceneVisibilityPrivate, makeConfigGeneralResult().DefaultSceneVisibility, "configured")

	// the new scene is hidden from the editor that creates it
	data := v.mustRun(models.RoleIDEditor, `mutation { sceneCreate(input: {title: "editor scene"}) { id } }`)
	assert.Nil(t, data["sceneCreate"], "scene returned to editor")

	data = v.mustRun(models.RoleIDEditor, `query { findScenes(scene_filter: {title: {value: "editor scene", modifier: EQUALS}}) { count } }`)
	assert.EqualValues(t, 0, data["findScenes"].(map[string]interface{})["count"], "editor finds the scene")

	data = v.mustRun(models.RoleIDAdmin, `query { findScenes(scene_filter: {title: {value: "editor scene", modifier: EQUALS}}) { scenes { visibility } } }`)
	scenes := data["findScenes"].(map[string]interface{})["scenes"].([]interface{})
	if assert.Len(t, scenes, 1, "admin finds the scene") {
		assert.Equal(t, "PRIVATE", scenes[0].(map[string]interface{})["visibility"])
	}

	data = v.mustRun(models.RoleIDAdmin, `mutation { sceneCreate(input: {title: "admin scene"}) { visibility } }`)
	assert.Equal(t, "PRIVATE", data["sceneCreate"].(map[string]interface{})["visibility"], "scene created by admin")

	// changing the default back applies to new scenes only
	v.cfg.SetString(config.DefaultSceneVisibility, models.SceneVisibilityEveryone.String())
	data = v.mustRun(models.RoleIDViewer, `query { findScenes { count } }`)
	assert.EqualValues(t, 1, data["findScenes"].(map[string]interface{})["count"])

	data = v.mustRun(models.RoleIDEditor, `mutation { sceneCreate(input: {title: "visible scene"}) { visibility } }`)
	assert.Equal(t, "EVERYONE", data["sceneCreate"].(map[string]interface{})["visibility"])
}
