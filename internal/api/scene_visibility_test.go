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

var sceneVisibilities = []models.RoleEnum{models.RoleEnumRead, models.RoleEnumModify, models.RoleEnumAdmin}

// sceneVisibilityTest runs GraphQL operations against a new database that has
// a scene of each visibility.
type sceneVisibilityTest struct {
	t    *testing.T
	cfg  *config.Config
	db   *sqlite.Database
	repo models.Repository
	exec *executor.Executor

	// scene ID and the oshash of its file, by the visibility of the scene
	sceneIDs map[models.RoleEnum]int
	hashes   map[models.RoleEnum]string
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
			HasRole:     HasRoleDirective,
			IsUserOwner: IsUserOwnerDirective,
		},
	}))
	exec.AroundRootFields(mutationRoleMiddleware)

	v := &sceneVisibilityTest{
		t:        t,
		cfg:      cfg,
		db:       db,
		repo:     repo,
		exec:     exec,
		sceneIDs: make(map[models.RoleEnum]int),
		hashes:   make(map[models.RoleEnum]string),
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

// userContext returns a context of a user with the role, or of no user if the
// role is empty.
func userContext(ctx context.Context, role models.RoleEnum) context.Context {
	if role == "" {
		return ctx
	}

	return models.SetCurrentUser(ctx, models.User{Username: "user", Roles: models.Roles{role}})
}

// run executes the operation as a user with the role, and returns the
// response data and errors.
func (v *sceneVisibilityTest) run(role models.RoleEnum, query string, args ...interface{}) (map[string]interface{}, []error) {
	v.t.Helper()

	// the request context of the server: the user and the dataloaders
	var ctx context.Context
	r := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	r = r.WithContext(userContext(r.Context(), role))
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
func (v *sceneVisibilityTest) mustRun(role models.RoleEnum, query string, args ...interface{}) map[string]interface{} {
	v.t.Helper()

	data, errs := v.run(role, query, args...)
	require.Empty(v.t, errs)
	return data
}

func (v *sceneVisibilityTest) visibility(id int) models.RoleEnum {
	v.t.Helper()

	var ret models.RoleEnum
	require.NoError(v.t, txn.WithReadTxn(context.Background(), v.db, func(ctx context.Context) error {
		var err error
		ret, err = v.db.Scene.GetVisibility(ctx, id)
		return err
	}))
	return ret
}

func assertDenied(t *testing.T, errs []error, role models.RoleEnum, msg string) {
	t.Helper()

	if assert.Len(t, errs, 1, msg) {
		assertPermissionDenied(t, errs[0], role)
	}
}

func TestSceneVisibilityFindScenes(t *testing.T) {
	v := newSceneVisibilityTest(t)

	tests := []struct {
		role    models.RoleEnum
		visible []models.RoleEnum
	}{
		{"", sceneVisibilities},
		{models.RoleEnumRead, []models.RoleEnum{models.RoleEnumRead}},
		{models.RoleEnumModify, []models.RoleEnum{models.RoleEnumRead, models.RoleEnumModify}},
		{models.RoleEnumAdmin, sceneVisibilities},
	}

	for _, tt := range tests {
		t.Run("user "+tt.role.String(), func(t *testing.T) {
			data := v.mustRun(tt.role, `query { findScenes { count scenes { id visibility } } }`)
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
				data := v.mustRun(tt.role, `query { findScene(id: "%d") { id } }`, v.sceneIDs[visibility])
				hidden := !slices.Contains(tt.visible, visibility)
				assert.Equal(t, hidden, data["findScene"] == nil, "findScene of %s scene", visibility)
			}
		})
	}
}

func TestSceneVisibilityAdminOnly(t *testing.T) {
	v := newSceneVisibilityTest(t)
	readID := v.sceneIDs[models.RoleEnumRead]

	// editors can update scenes, but not their visibility
	editor := models.RoleEnumModify
	_, errs := v.run(editor, `mutation { sceneUpdate(input: {id: "%d", visibility: ADMIN}) { id } }`, readID)
	assertDenied(t, errs, models.RoleEnumAdmin, "sceneUpdate")

	_, errs = v.run(editor, `mutation { bulkSceneUpdate(input: {ids: ["%d"], visibility: ADMIN}) { id } }`, readID)
	assertDenied(t, errs, models.RoleEnumAdmin, "bulkSceneUpdate")

	_, errs = v.run(editor, `mutation { scenesUpdate(input: [{id: "%d", visibility: ADMIN}]) { id } }`, readID)
	assertDenied(t, errs, models.RoleEnumAdmin, "scenesUpdate")

	assert.Equal(t, models.RoleEnumRead, v.visibility(readID), "visibility after denied changes")

	// anyone can filter by visibility, which only matches the scenes they can see
	data := v.mustRun(editor, `query { findScenes(scene_filter: {visibility: {value: [ADMIN], modifier: INCLUDES}}) { count } }`)
	assert.EqualValues(t, 0, data["findScenes"].(map[string]interface{})["count"], "editor filtering admin scenes")

	data = v.mustRun(editor, `query { findScenes(scene_filter: {visibility: {value: [MODIFY], modifier: INCLUDES}}) { count } }`)
	assert.EqualValues(t, 1, data["findScenes"].(map[string]interface{})["count"], "editor filtering editor scenes")

	// editing other fields is not restricted
	v.mustRun(models.RoleEnumModify, `mutation { sceneUpdate(input: {id: "%d", title: "changed"}) { id } }`, readID)

	data = v.mustRun(models.RoleEnumAdmin, `query { findScenes(scene_filter: {visibility: {value: [ADMIN], modifier: INCLUDES}}) { count } }`)
	assert.EqualValues(t, 1, data["findScenes"].(map[string]interface{})["count"])

	data = v.mustRun(models.RoleEnumAdmin, `mutation { sceneUpdate(input: {id: "%d", visibility: MODIFY}) { visibility } }`, readID)
	assert.Equal(t, "MODIFY", data["sceneUpdate"].(map[string]interface{})["visibility"])
	assert.Equal(t, models.RoleEnumModify, v.visibility(readID))

	v.mustRun(models.RoleEnumAdmin, `mutation { bulkSceneUpdate(input: {ids: ["%d"], visibility: ADMIN}) { id } }`, readID)
	assert.Equal(t, models.RoleEnumAdmin, v.visibility(readID))

	v.mustRun(models.RoleEnumAdmin, `mutation { scenesUpdate(input: [{id: "%d", visibility: READ}]) { id } }`, readID)
	assert.Equal(t, models.RoleEnumRead, v.visibility(readID))
}

var notFoundRE = regexp.MustCompile("not found|does not exist")

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

	hiddenID := v.sceneIDs[models.RoleEnumAdmin]
	visibleID := v.sceneIDs[models.RoleEnumModify]

	for _, m := range mutations {
		t.Run(m.name, func(t *testing.T) {
			_, errs := v.run(models.RoleEnumModify, m.query, hiddenID)
			if assert.Len(t, errs, 1, "hidden scene") {
				assert.Regexp(t, notFoundRE, errs[0].Error())
			}

			if m.runsVisible {
				_, errs = v.run(models.RoleEnumModify, m.query, visibleID)
				assert.Empty(t, errs, "visible scene")
			}
		})
	}

	// the hidden scene is unchanged
	data := v.mustRun(models.RoleEnumAdmin, `query { findScene(id: "%d") { title o_counter play_count resume_time scene_markers { id } } }`, hiddenID)
	s := data["findScene"].(map[string]interface{})
	assert.Equal(t, "ADMIN", s["title"])
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

	serve := func(router http.Handler, role models.RoleEnum, target string) int {
		r := httptest.NewRequest(http.MethodGet, target, nil)
		r = r.WithContext(userContext(r.Context(), role))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w.Code
	}

	viewerHidden := v.sceneIDs[models.RoleEnumModify]
	viewerHash := v.hashes[models.RoleEnumModify]

	// the routes of the server are checked
	routes := rs.Routes()
	for _, target := range []string{
		fmt.Sprintf("/%d/screenshot", viewerHidden),
		fmt.Sprintf("/%d/stream", viewerHidden),
		fmt.Sprintf("/%d/vtt/chapter", viewerHidden),
		fmt.Sprintf("/%s_thumbs.vtt", viewerHash),
		fmt.Sprintf("/%s_sprite.jpg", viewerHash),
	} {
		assert.Equal(t, http.StatusNotFound, serve(routes, models.RoleEnumRead, target), "viewer %s", target)
	}

	tests := []struct {
		role       models.RoleEnum
		visibility models.RoleEnum
		want       int
	}{
		{models.RoleEnumRead, models.RoleEnumRead, http.StatusOK},
		{models.RoleEnumRead, models.RoleEnumModify, http.StatusNotFound},
		{models.RoleEnumModify, models.RoleEnumModify, http.StatusOK},
		{models.RoleEnumModify, models.RoleEnumAdmin, http.StatusNotFound},
		{models.RoleEnumAdmin, models.RoleEnumAdmin, http.StatusOK},
		{"", models.RoleEnumAdmin, http.StatusOK},
	}

	for _, tt := range tests {
		id := v.sceneIDs[tt.visibility]
		assert.Equal(t, tt.want, serve(checked, tt.role, fmt.Sprintf("/%d/screenshot", id)), "user %q, %s scene by id", tt.role, tt.visibility)

		hash := v.hashes[tt.visibility]
		assert.Equal(t, tt.want, serve(checked, tt.role, fmt.Sprintf("/%s_thumbs.vtt", hash)), "user %q, %s scene by hash", tt.role, tt.visibility)
	}

	assert.Equal(t, http.StatusNotFound, serve(checked, models.RoleEnumAdmin, "/unknown_thumbs.vtt"), "unknown hash")
}

func TestSceneVisibilityDefault(t *testing.T) {
	v := newSceneVisibilityTest(t)

	assert.Equal(t, models.RoleEnumRead, makeConfigGeneralResult().DefaultSceneVisibility, "default")

	v.cfg.SetString(config.DefaultSceneVisibility, models.RoleEnumAdmin.String())
	assert.Equal(t, models.RoleEnumAdmin, makeConfigGeneralResult().DefaultSceneVisibility, "configured")

	// the new scene is hidden from the editor that creates it
	data := v.mustRun(models.RoleEnumModify, `mutation { sceneCreate(input: {title: "editor scene"}) { id } }`)
	assert.Nil(t, data["sceneCreate"], "scene returned to editor")

	data = v.mustRun(models.RoleEnumModify, `query { findScenes(scene_filter: {title: {value: "editor scene", modifier: EQUALS}}) { count } }`)
	assert.EqualValues(t, 0, data["findScenes"].(map[string]interface{})["count"], "editor finds the scene")

	data = v.mustRun(models.RoleEnumAdmin, `query { findScenes(scene_filter: {title: {value: "editor scene", modifier: EQUALS}}) { scenes { visibility } } }`)
	scenes := data["findScenes"].(map[string]interface{})["scenes"].([]interface{})
	if assert.Len(t, scenes, 1, "admin finds the scene") {
		assert.Equal(t, "ADMIN", scenes[0].(map[string]interface{})["visibility"])
	}

	data = v.mustRun(models.RoleEnumAdmin, `mutation { sceneCreate(input: {title: "admin scene"}) { visibility } }`)
	assert.Equal(t, "ADMIN", data["sceneCreate"].(map[string]interface{})["visibility"], "scene created by admin")

	// changing the default back applies to new scenes only
	v.cfg.SetString(config.DefaultSceneVisibility, models.RoleEnumRead.String())
	data = v.mustRun(models.RoleEnumRead, `query { findScenes { count } }`)
	assert.EqualValues(t, 1, data["findScenes"].(map[string]interface{})["count"])

	data = v.mustRun(models.RoleEnumModify, `mutation { sceneCreate(input: {title: "visible scene"}) { visibility } }`)
	assert.Equal(t, "READ", data["sceneCreate"].(map[string]interface{})["visibility"])
}
