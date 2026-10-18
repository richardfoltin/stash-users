package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// Requirements of each mutation: a permission, ADMIN for the Admin role, or
// SIGNED_IN for every signed in user.
//
// RECORD_ACTIVITY: recording their own viewing activity (o-counter, play
// history, resume point and play duration).
// The object permissions: creating and editing (_EDIT), deleting (_DELETE)
// and merging (_MERGE) each kind of object.
// ADMIN: everything else - configuration, the API key, plugins and packages,
// import and export, database maintenance, SQL, DLNA, stash-box submissions,
// and user and role management.
//
// Every mutation in the schema must be listed here. When a mutation is added
// to the schema, it must be classified here deliberately.
var mutationRequirements = map[string]string{
	// setup and system
	"setup":          "ADMIN",
	"migrate":        "ADMIN",
	"downloadFFMpeg": "ADMIN",

	// scenes
	"sceneCreate":             "SCENES_EDIT",
	"sceneUpdate":             "SCENES_EDIT",
	"sceneMerge":              "SCENES_MERGE",
	"bulkSceneUpdate":         "SCENES_EDIT",
	"sceneDestroy":            "SCENES_DELETE",
	"scenesDestroy":           "SCENES_DELETE",
	"scenesUpdate":            "SCENES_EDIT",
	"sceneIncrementO":         "RECORD_ACTIVITY",
	"sceneDecrementO":         "RECORD_ACTIVITY",
	"sceneAddO":               "RECORD_ACTIVITY",
	"sceneDeleteO":            "RECORD_ACTIVITY",
	"sceneResetO":             "RECORD_ACTIVITY",
	"sceneSaveActivity":       "RECORD_ACTIVITY",
	"sceneResetActivity":      "RECORD_ACTIVITY",
	"sceneIncrementPlayCount": "RECORD_ACTIVITY",
	"sceneAddPlay":            "RECORD_ACTIVITY",
	"sceneDeletePlay":         "RECORD_ACTIVITY",
	"sceneResetPlayCount":     "RECORD_ACTIVITY",
	"sceneGenerateScreenshot": "SCENES_EDIT",
	"sceneAssignFile":         "MANAGE_FILES",

	// scene markers
	"sceneMarkerCreate":     "MARKERS_EDIT",
	"sceneMarkerUpdate":     "MARKERS_EDIT",
	"bulkSceneMarkerUpdate": "MARKERS_EDIT",
	"sceneMarkerDestroy":    "MARKERS_DELETE",
	"sceneMarkersDestroy":   "MARKERS_DELETE",

	// images
	"imageUpdate":     "IMAGES_EDIT",
	"bulkImageUpdate": "IMAGES_EDIT",
	"imageDestroy":    "IMAGES_DELETE",
	"imagesDestroy":   "IMAGES_DELETE",
	"imagesUpdate":    "IMAGES_EDIT",
	"imageIncrementO": "RECORD_ACTIVITY",
	"imageDecrementO": "RECORD_ACTIVITY",
	"imageResetO":     "RECORD_ACTIVITY",

	// galleries
	"galleryCreate":         "GALLERIES_EDIT",
	"galleryUpdate":         "GALLERIES_EDIT",
	"bulkGalleryUpdate":     "GALLERIES_EDIT",
	"galleryDestroy":        "GALLERIES_DELETE",
	"galleriesUpdate":       "GALLERIES_EDIT",
	"addGalleryImages":      "GALLERIES_EDIT",
	"removeGalleryImages":   "GALLERIES_EDIT",
	"setGalleryCover":       "GALLERIES_EDIT",
	"resetGalleryCover":     "GALLERIES_EDIT",
	"galleryChapterCreate":  "CHAPTERS_EDIT",
	"galleryChapterUpdate":  "CHAPTERS_EDIT",
	"galleryChapterDestroy": "CHAPTERS_DELETE",

	// performers
	"performerCreate":     "PERFORMERS_EDIT",
	"performerUpdate":     "PERFORMERS_EDIT",
	"performerDestroy":    "PERFORMERS_DELETE",
	"performersDestroy":   "PERFORMERS_DELETE",
	"bulkPerformerUpdate": "PERFORMERS_EDIT",
	"performerMerge":      "PERFORMERS_MERGE",

	// studios
	"studioCreate":     "STUDIOS_EDIT",
	"studioUpdate":     "STUDIOS_EDIT",
	"studioDestroy":    "STUDIOS_DELETE",
	"studiosDestroy":   "STUDIOS_DELETE",
	"bulkStudioUpdate": "STUDIOS_EDIT",

	// movies (deprecated) and groups
	"movieCreate":          "GROUPS_EDIT",
	"movieUpdate":          "GROUPS_EDIT",
	"movieDestroy":         "GROUPS_DELETE",
	"moviesDestroy":        "GROUPS_DELETE",
	"bulkMovieUpdate":      "GROUPS_EDIT",
	"groupCreate":          "GROUPS_EDIT",
	"groupUpdate":          "GROUPS_EDIT",
	"groupDestroy":         "GROUPS_DELETE",
	"groupsDestroy":        "GROUPS_DELETE",
	"bulkGroupUpdate":      "GROUPS_EDIT",
	"addGroupSubGroups":    "GROUPS_EDIT",
	"removeGroupSubGroups": "GROUPS_EDIT",
	"reorderSubGroups":     "GROUPS_EDIT",

	// tags
	"tagCreate":     "TAGS_EDIT",
	"tagUpdate":     "TAGS_EDIT",
	"tagDestroy":    "TAGS_DELETE",
	"tagsDestroy":   "TAGS_DELETE",
	"tagsMerge":     "TAGS_MERGE",
	"bulkTagUpdate": "TAGS_EDIT",

	// files
	"moveFiles":                 "MANAGE_FILES",
	"deleteFiles":               "MANAGE_FILES",
	"destroyFiles":              "MANAGE_FILES",
	"fileSetFingerprints":       "MANAGE_FILES",
	"revealFileInFileManager":   "MANAGE_FILES",
	"revealFolderInFileManager": "MANAGE_FILES",

	// saved filters
	"saveFilter":         "SAVE_FILTERS",
	"destroySavedFilter": "DELETE_SAVED_FILTERS",
	"setDefaultFilter":   "MANAGE_SHARED_LIST_SETTINGS",

	// configuration
	"configureGeneral":  "ADMIN",
	"configureDLNA":     "ADMIN",
	"configureScraping": "ADMIN",
	"configureDefaults": "ADMIN",
	"configurePlugin":   "ADMIN",
	"generateAPIKey":    "ADMIN",

	// interface settings: the shared fields and keys are checked by the
	// resolvers
	"configureInterface":      "SIGNED_IN",
	"configureUI":             "SIGNED_IN",
	"configureUISetting":      "SIGNED_IN",
	"resetPersonalSettings":   "SIGNED_IN",
	"publishPersonalSettings": "SIGNED_IN",

	// tasks and jobs
	"exportObjects":           "ADMIN",
	"importObjects":           "ADMIN",
	"metadataImport":          "ADMIN",
	"metadataExport":          "ADMIN",
	"metadataScan":            "RUN_LIBRARY_TASKS",
	"metadataGenerate":        "RUN_LIBRARY_TASKS",
	"metadataAutoTag":         "RUN_LIBRARY_TASKS",
	"metadataClean":           "RUN_LIBRARY_TASKS",
	"metadataCleanGenerated":  "RUN_LIBRARY_TASKS",
	"metadataIdentify":        "RUN_LIBRARY_TASKS",
	"migrateHashNaming":       "ADMIN",
	"migrateSceneScreenshots": "ADMIN",
	"migrateBlobs":            "ADMIN",
	"anonymiseDatabase":       "ADMIN",
	"optimiseDatabase":        "ADMIN",
	"backupDatabase":          "ADMIN",
	"stopJob":                 "RUN_LIBRARY_TASKS",
	"stopAllJobs":             "RUN_LIBRARY_TASKS",

	// scrapers, plugins and packages
	"reloadScrapers":     "ADMIN",
	"setPluginsEnabled":  "ADMIN",
	"runPluginTask":      "ADMIN",
	"runPluginOperation": "ADMIN",
	"reloadPlugins":      "ADMIN",
	"installPackages":    "ADMIN",
	"updatePackages":     "ADMIN",
	"uninstallPackages":  "ADMIN",

	// stash-box
	"submitStashBoxFingerprints":   "ADMIN",
	"submitStashBoxSceneDraft":     "ADMIN",
	"submitStashBoxPerformerDraft": "ADMIN",
	"stashBoxBatchPerformerTag":    "ADMIN",
	"stashBoxBatchStudioTag":       "ADMIN",
	"stashBoxBatchTagTag":          "ADMIN",

	// raw SQL
	"querySQL": "ADMIN",
	"execSQL":  "ADMIN",

	// DLNA
	"enableDLNA":       "ADMIN",
	"disableDLNA":      "ADMIN",
	"addTempDLNAIP":    "ADMIN",
	"removeTempDLNAIP": "ADMIN",

	// users
	"userCreate":         "ADMIN",
	"userUpdate":         "ADMIN",
	"userDestroy":        "ADMIN",
	"changeUserPassword": "ADMIN",
	"changePassword":     "SIGNED_IN",

	// roles
	"roleCreate":  "ADMIN",
	"roleUpdate":  "ADMIN",
	"roleDestroy": "ADMIN",
}

// Queries and subscriptions that require more than being signed in. All other
// queries and subscriptions are available to every user.
var queryRequirements = map[string]string{
	"personalSettings":            "SIGNED_IN",
	"users":                       "ADMIN",
	"roles":                       "ADMIN",
	"logs":                        "ADMIN",
	"directory":                   "ADMIN",
	"installedPackages":           "ADMIN",
	"availablePackages":           "ADMIN",
	"validateStashBoxCredentials": "ADMIN",

	"scrapeSingleScene":     "USE_SCRAPERS",
	"scrapeMultiScenes":     "USE_SCRAPERS",
	"scrapeSingleStudio":    "USE_SCRAPERS",
	"scrapeSingleTag":       "USE_SCRAPERS",
	"scrapeSinglePerformer": "USE_SCRAPERS",
	"scrapeMultiPerformers": "USE_SCRAPERS",
	"scrapeSingleGallery":   "USE_SCRAPERS",
	"scrapeSingleMovie":     "USE_SCRAPERS",
	"scrapeSingleGroup":     "USE_SCRAPERS",
	"scrapeSingleImage":     "USE_SCRAPERS",
	"scrapeURL":             "USE_SCRAPERS",
	"scrapePerformerURL":    "USE_SCRAPERS",
	"scrapeSceneURL":        "USE_SCRAPERS",
	"scrapeGalleryURL":      "USE_SCRAPERS",
	"scrapeImageURL":        "USE_SCRAPERS",
	"scrapeMovieURL":        "USE_SCRAPERS",
	"scrapeGroupURL":        "USE_SCRAPERS",

	// subscriptions
	"loggingSubscribe": "ADMIN",
}

// Input fields that require a permission to set.
var inputFieldRequirements = map[string]string{
	"SceneUpdateInput.visibility":     "CHANGE_SCENE_VISIBILITY",
	"BulkSceneUpdateInput.visibility": "CHANGE_SCENE_VISIBILITY",
	"SceneDestroyInput.delete_file":   "MANAGE_FILES",
	"ScenesDestroyInput.delete_file":  "MANAGE_FILES",
	"ImageDestroyInput.delete_file":   "MANAGE_FILES",
	"ImagesDestroyInput.delete_file":  "MANAGE_FILES",
	"GalleryDestroyInput.delete_file": "MANAGE_FILES",
}

// requirementName returns the name of the requirement, as in the tables.
func requirementName(req requirement) string {
	switch {
	case req.adminOnly:
		return "ADMIN"
	case req.signedIn:
		return "SIGNED_IN"
	}

	return req.permission.String()
}

func loadTestSchema(t *testing.T) *ast.Schema {
	t.Helper()

	var files []string
	for _, pattern := range []string{"../../graphql/schema/types/*.graphql", "../../graphql/schema/*.graphql"} {
		matches, err := filepath.Glob(pattern)
		require.NoError(t, err)
		files = append(files, matches...)
	}
	require.NotEmpty(t, files)

	var sources []*ast.Source
	for _, fn := range files {
		data, err := os.ReadFile(fn)
		require.NoError(t, err)
		sources = append(sources, &ast.Source{Name: fn, Input: string(data)})
	}

	schema, gqlErr := gqlparser.LoadSchema(sources...)
	if gqlErr != nil {
		require.NoError(t, gqlErr)
	}

	return schema
}

func schemaFields(def *ast.Definition) []*ast.FieldDefinition {
	var ret []*ast.FieldDefinition
	for _, f := range def.Fields {
		if strings.HasPrefix(f.Name, "__") {
			continue
		}
		ret = append(ret, f)
	}
	return ret
}

// TestMutationRequirements classifies every mutation of the schema by
// requirement. It fails when a mutation is not classified, or is classified
// differently.
func TestMutationRequirements(t *testing.T) {
	schema := loadTestSchema(t)
	require.NotNil(t, schema.Mutation)

	inSchema := make(map[string]bool)
	for _, f := range schemaFields(schema.Mutation) {
		inSchema[f.Name] = true

		t.Run(f.Name, func(t *testing.T) {
			expected, classified := mutationRequirements[f.Name]
			if !assert.True(t, classified, "mutation %q is not classified: add it to mutationRequirements", f.Name) {
				return
			}

			req, hasDirective := fieldRequirement(f)
			assert.True(t, hasDirective, "mutation %q must have an explicit @hasPermission, @adminOnly or @signedIn directive", f.Name)
			if !req.adminOnly && !req.signedIn {
				assert.True(t, req.permission.IsValid(), "mutation %q has an invalid permission %q", f.Name, req.permission)
			}

			// the requirement that is enforced at runtime
			assert.Equal(t, expected, requirementName(mutationRequirement(f)), "mutation %q has the wrong requirement", f.Name)
		})
	}

	var stale []string
	for name := range mutationRequirements {
		if !inSchema[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	assert.Empty(t, stale, "classified mutations that are not in the schema")
}

// TestQueryRequirements ensures that only the expected queries and
// subscriptions are restricted, and that their requirements are as expected.
func TestQueryRequirements(t *testing.T) {
	schema := loadTestSchema(t)

	seen := make(map[string]bool)
	for _, def := range []*ast.Definition{schema.Query, schema.Subscription} {
		require.NotNil(t, def)
		for _, f := range schemaFields(def) {
			req, restricted := fieldRequirement(f)
			expected, expectRestricted := queryRequirements[f.Name]
			seen[f.Name] = true

			if !expectRestricted {
				assert.False(t, restricted, "%s.%s is unexpectedly restricted to %s", def.Name, f.Name, requirementName(req))
				continue
			}

			assert.True(t, restricted, "%s.%s must be restricted to %s", def.Name, f.Name, expected)
			assert.Equal(t, expected, requirementName(req), "%s.%s has the wrong requirement", def.Name, f.Name)
		}
	}

	for name := range queryRequirements {
		assert.True(t, seen[name], "restricted field %q is not in the schema", name)
	}
}

// TestInputFieldRequirements ensures that only the expected input fields are
// restricted.
func TestInputFieldRequirements(t *testing.T) {
	schema := loadTestSchema(t)

	found := make(map[string]string)
	for _, def := range schema.Types {
		if def.Kind != ast.InputObject {
			continue
		}

		for _, f := range def.Fields {
			if req, ok := fieldRequirement(f); ok {
				found[def.Name+"."+f.Name] = requirementName(req)
			}
		}
	}

	assert.Equal(t, inputFieldRequirements, found)
}

// TestAllPermissionsRoleDeniedAdminOnly checks that a role with every
// permission is still denied the admin only operations, such as the users
// and roles queries. The custom JavaScript, which every browser runs, is
// checked by the configureInterface resolver.
func TestAllPermissionsRoleDeniedAdminOnly(t *testing.T) {
	schema := loadTestSchema(t)
	ctx := models.SetCurrentUser(context.Background(), models.User{
		Username:    "everything",
		Role:        "everything",
		Permissions: models.AllPermissions,
	})

	denied := make(map[string]bool)
	for _, def := range []*ast.Definition{schema.Query, schema.Mutation, schema.Subscription} {
		for _, f := range schemaFields(def) {
			req, restricted := fieldRequirement(f)
			if def == schema.Mutation {
				req, restricted = mutationRequirement(f), true
			}

			if !restricted {
				continue
			}

			err := checkRequirement(ctx, req)
			if req.adminOnly {
				assertPermissionDenied(t, err, req)
				denied[f.Name] = true
			} else {
				assert.NoError(t, err, "%s.%s", def.Name, f.Name)
			}
		}
	}

	var adminOnly []string
	for _, table := range []map[string]string{mutationRequirements, queryRequirements} {
		for name, req := range table {
			if req == "ADMIN" {
				adminOnly = append(adminOnly, name)
			}
		}
	}
	assert.Len(t, denied, len(adminOnly))
	for _, name := range []string{"users", "roles", "roleCreate", "userCreate", "generateAPIKey"} {
		assert.True(t, denied[name], "%s is denied", name)
	}
}

func TestCheckRequirement(t *testing.T) {
	defaults := models.DefaultRoles()
	editor := models.User{Username: "editor", Role: models.RoleIDEditor, Permissions: defaults[0].Permissions}
	viewer := models.User{Username: "viewer", Role: models.RoleIDViewer, Permissions: defaults[1].Permissions}
	admin := models.User{Username: "admin", Role: models.RoleIDAdmin}
	locked := models.User{Username: "locked"}

	editTags := requirement{permission: models.PermissionTagsEdit}
	deleteTags := requirement{permission: models.PermissionTagsDelete}
	recordActivity := requirement{permission: models.PermissionRecordActivity}
	signedIn := requirement{signedIn: true}

	tests := []struct {
		name    string
		user    models.User
		req     requirement
		allowed bool
	}{
		{"viewer records activity", viewer, recordActivity, true},
		{"viewer edits tags", viewer, editTags, false},
		{"viewer is signed in", viewer, signedIn, true},
		{"editor edits tags", editor, editTags, true},
		{"editor deletes tags", editor, deleteTags, false},
		{"editor admin only", editor, adminRequirement, false},
		{"admin deletes tags", admin, deleteTags, true},
		{"admin admin only", admin, adminRequirement, true},
		{"locked records activity", locked, recordActivity, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkRequirement(models.SetCurrentUser(context.Background(), tt.user), tt.req)
			if tt.allowed {
				assert.NoError(t, err)
			} else {
				assertPermissionDenied(t, err, tt.req)
			}
		})
	}

	// without a current user, authentication is not configured
	assert.NoError(t, checkRequirement(context.Background(), adminRequirement))
}

// TestInputFieldDirective checks that an input field is checked only when it
// is set: false and null values are allowed.
func TestInputFieldDirective(t *testing.T) {
	ctx := models.SetCurrentUser(context.Background(), models.User{Username: "viewer", Role: models.RoleIDViewer})
	input := map[string]interface{}{}

	unset := false
	set := true
	private := models.SceneVisibilityPrivate

	tests := []struct {
		name    string
		value   interface{}
		allowed bool
	}{
		{"null", (*bool)(nil), true},
		{"false", &unset, true},
		{"true", &set, false},
		{"value", &private, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := HasPermissionDirective(ctx, input, func(ctx context.Context) (interface{}, error) {
				return tt.value, nil
			}, models.PermissionManageFiles)

			if tt.allowed {
				assert.NoError(t, err)
				assert.Equal(t, tt.value, got)
			} else {
				assertPermissionDenied(t, err, requirement{permission: models.PermissionManageFiles})
			}
		})
	}
}

func TestIsUserOwnerDirective(t *testing.T) {
	withUser := func(name string, roleID string) context.Context {
		return models.SetCurrentUser(context.Background(), models.User{Username: name, Role: roleID})
	}

	owner := &models.User{Username: "viewer", Role: models.RoleIDViewer}

	tests := []struct {
		name    string
		ctx     context.Context
		obj     interface{}
		allowed bool
	}{
		{"no current user", context.Background(), owner, true},
		{"owner", withUser("viewer", models.RoleIDViewer), owner, true},
		{"other user", withUser("editor", models.RoleIDEditor), owner, false},
		{"admin", withUser("admin", models.RoleIDAdmin), owner, true},
		{"not a user", withUser("viewer", models.RoleIDViewer), "viewer", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			_, err := IsUserOwnerDirective(tt.ctx, tt.obj, func(ctx context.Context) (interface{}, error) {
				called = true
				return nil, nil
			})

			assert.Equal(t, tt.allowed, called)
			assert.Equal(t, tt.allowed, err == nil, "error: %v", err)
		})
	}
}

func TestIsPermissionDenied(t *testing.T) {
	assert.True(t, isPermissionDenied(newPermissionDeniedError(adminRequirement)))
	assert.True(t, isPermissionDenied(fmt.Errorf("resolving: %w", newPermissionDeniedError(requirement{permission: models.PermissionTagsEdit}))))
	assert.False(t, isPermissionDenied(errors.New("permission denied")))
	assert.False(t, isPermissionDenied(&gqlerror.Error{Message: "other"}))
}

func assertPermissionDenied(t *testing.T, err error, req requirement) {
	t.Helper()

	var gqlErr *gqlerror.Error
	if !assert.True(t, errors.As(err, &gqlErr), "expected gqlerror, got %v", err) {
		return
	}
	assert.Equal(t, errCodePermissionDenied, gqlErr.Extensions["code"])
	if req.adminOnly {
		assert.Equal(t, models.RoleIDAdmin, gqlErr.Extensions["requiredRole"])
		assert.Contains(t, gqlErr.Message, "requires the Admin role")
	} else {
		assert.Equal(t, req.permission.String(), gqlErr.Extensions["requiredPermission"])
		assert.Contains(t, gqlErr.Message, "requires the "+req.permission.String()+" permission")
	}
	assert.Contains(t, gqlErr.Message, "permission denied")
}

// TestMutationRequirementMiddleware tests, through the gqlgen executor and the
// generated schema, that mutations require their permissions, and that
// mutations without a directive are restricted to the Admin role.
func TestMutationRequirementMiddleware(t *testing.T) {
	// a private copy of the schema, which the test changes
	schema := gqlparser.MustLoadSchema(sources...)
	es := NewExecutableSchema(Config{
		Schema:    schema,
		Resolvers: &Resolver{},
		Directives: DirectiveRoot{
			HasPermission: HasPermissionDirective,
			AdminOnly:     AdminOnlyDirective,
			SignedIn:      SignedInDirective,
			IsUserOwner:   IsUserOwnerDirective,
		},
	})

	exec := executor.New(es)
	exec.AroundRootFields(mutationRequirementMiddleware)
	// stands in for the resolvers: a root field resolves as soon as the
	// requirement check passes, so the resolvers (and their directives) are
	// not called
	exec.AroundRootFields(func(ctx context.Context, next graphql.RootResolver) graphql.Marshaler {
		return graphql.MarshalBoolean(true)
	})

	const tagCreate = `mutation { tagCreate(input: {name: "tag"}) { id } }`
	const tagDestroy = `mutation { tagDestroy(input: {id: "1"}) }`
	const findTags = `query { findTags { count } }`
	const sceneAddO = `mutation { sceneAddO(id: "1") { count } }`
	const sceneSaveActivity = `mutation { sceneSaveActivity(id: "1", resume_time: 1) }`
	const imageIncrementO = `mutation { imageIncrementO(id: "1") }`
	const sceneRate = `mutation { sceneUpdate(input: {id: "1", rating100: 50}) { id } }`

	// run executes the operation and returns whether its root field was
	// resolved, and the requirement of a permission denied error
	run := func(t *testing.T, query string, u *models.User) (resolved bool, denied string) {
		t.Helper()

		ctx := graphql.StartOperationTrace(context.Background())
		if u != nil {
			ctx = models.SetCurrentUser(ctx, *u)
		}

		opCtx, errs := exec.CreateOperationContext(ctx, &graphql.RawParams{Query: query})
		require.Empty(t, errs)

		handler, ctx := exec.DispatchOperation(ctx, opCtx)
		resp := handler(ctx)
		require.NotNil(t, resp)

		if len(resp.Errors) > 0 {
			require.Len(t, resp.Errors, 1)
			ext := resp.Errors[0].Extensions
			require.Equal(t, errCodePermissionDenied, ext["code"], resp.Errors[0].Message)
			denied, _ = ext["requiredPermission"].(string)
			if role, ok := ext["requiredRole"].(string); ok {
				denied = role
			}
		}

		return strings.Contains(string(resp.Data), "true"), denied
	}

	defaults := models.DefaultRoles()
	editor := &models.User{Username: "editor", Role: models.RoleIDEditor, Permissions: defaults[0].Permissions}
	viewer := &models.User{Username: "viewer", Role: models.RoleIDViewer, Permissions: defaults[1].Permissions}
	tagDeleter := &models.User{Username: "deleter", Role: "custom", Permissions: models.Permissions{models.PermissionTagsDelete}}
	admin := &models.User{Username: "admin", Role: models.RoleIDAdmin}

	type testCase struct {
		name     string
		query    string
		user     *models.User
		resolved bool
		denied   string
	}

	runTests := func(t *testing.T, tests []testCase) {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				resolved, denied := run(t, tt.query, tt.user)
				assert.Equal(t, tt.resolved, resolved, "resolved")
				assert.Equal(t, tt.denied, denied, "requirement")
			})
		}
	}

	// tagCreate requires TAGS_EDIT in the schema
	t.Run("annotated", func(t *testing.T) {
		runTests(t, []testCase{
			{"mutation as viewer", tagCreate, viewer, false, "TAGS_EDIT"},
			{"mutation as editor", tagCreate, editor, true, ""},
			{"mutation as admin", tagCreate, admin, true, ""},
			{"delete as editor", tagDestroy, editor, false, "TAGS_DELETE"},
			{"delete with the permission", tagDestroy, tagDeleter, true, ""},
			{"edit without the permission", tagCreate, tagDeleter, false, "TAGS_EDIT"},
			{"query as viewer", findTags, viewer, true, ""},
			// viewing activity is personal: viewers record their own
			{"activity as viewer", sceneAddO, viewer, true, ""},
			{"resume point as viewer", sceneSaveActivity, viewer, true, ""},
			{"image activity as viewer", imageIncrementO, viewer, true, ""},
			{"rating as viewer", sceneRate, viewer, false, "SCENES_EDIT"},
		})
	})

	// remove the directive of tagCreate, to simulate a mutation added without
	// one
	def := schema.Mutation.Fields.ForName("tagCreate")
	require.NotNil(t, def)
	def.Directives = nil

	t.Run("unannotated", func(t *testing.T) {
		runTests(t, []testCase{
			{"mutation as viewer", tagCreate, viewer, false, "ADMIN"},
			{"mutation as editor", tagCreate, editor, false, "ADMIN"},
			{"mutation as admin", tagCreate, admin, true, ""},
			{"mutation without authentication", tagCreate, nil, true, ""},
		})
	})
}
