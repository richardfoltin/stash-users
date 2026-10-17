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

// Roles required for each mutation.
//
// READ (Viewer): no mutations, other than changing their own password.
// MODIFY (Editor): creating and editing library metadata, ratings, o-counter,
// play history and resume points, generating a cover screenshot.
// ADMIN: everything else - destroying, deleting and merging, configuration,
// tasks and jobs, plugins and packages, SQL, DLNA, stash-box submissions and
// user management.
//
// Every mutation in the schema must be listed here. When a mutation is added
// to the schema, it must be classified here deliberately.
var expectedMutationRoles = map[string]models.RoleEnum{
	// setup and system
	"setup":          models.RoleEnumAdmin,
	"migrate":        models.RoleEnumAdmin,
	"downloadFFMpeg": models.RoleEnumAdmin,

	// scenes
	"sceneCreate":             models.RoleEnumModify,
	"sceneUpdate":             models.RoleEnumModify,
	"sceneMerge":              models.RoleEnumAdmin,
	"bulkSceneUpdate":         models.RoleEnumModify,
	"sceneDestroy":            models.RoleEnumAdmin,
	"scenesDestroy":           models.RoleEnumAdmin,
	"scenesUpdate":            models.RoleEnumModify,
	"sceneIncrementO":         models.RoleEnumModify,
	"sceneDecrementO":         models.RoleEnumModify,
	"sceneAddO":               models.RoleEnumModify,
	"sceneDeleteO":            models.RoleEnumModify,
	"sceneResetO":             models.RoleEnumModify,
	"sceneSaveActivity":       models.RoleEnumModify,
	"sceneResetActivity":      models.RoleEnumModify,
	"sceneIncrementPlayCount": models.RoleEnumModify,
	"sceneAddPlay":            models.RoleEnumModify,
	"sceneDeletePlay":         models.RoleEnumModify,
	"sceneResetPlayCount":     models.RoleEnumModify,
	"sceneGenerateScreenshot": models.RoleEnumModify,
	"sceneAssignFile":         models.RoleEnumAdmin,

	// scene markers
	"sceneMarkerCreate":     models.RoleEnumModify,
	"sceneMarkerUpdate":     models.RoleEnumModify,
	"bulkSceneMarkerUpdate": models.RoleEnumModify,
	"sceneMarkerDestroy":    models.RoleEnumAdmin,
	"sceneMarkersDestroy":   models.RoleEnumAdmin,

	// images
	"imageUpdate":     models.RoleEnumModify,
	"bulkImageUpdate": models.RoleEnumModify,
	"imageDestroy":    models.RoleEnumAdmin,
	"imagesDestroy":   models.RoleEnumAdmin,
	"imagesUpdate":    models.RoleEnumModify,
	"imageIncrementO": models.RoleEnumModify,
	"imageDecrementO": models.RoleEnumModify,
	"imageResetO":     models.RoleEnumModify,

	// galleries
	"galleryCreate":         models.RoleEnumModify,
	"galleryUpdate":         models.RoleEnumModify,
	"bulkGalleryUpdate":     models.RoleEnumModify,
	"galleryDestroy":        models.RoleEnumAdmin,
	"galleriesUpdate":       models.RoleEnumModify,
	"addGalleryImages":      models.RoleEnumModify,
	"removeGalleryImages":   models.RoleEnumModify,
	"setGalleryCover":       models.RoleEnumModify,
	"resetGalleryCover":     models.RoleEnumModify,
	"galleryChapterCreate":  models.RoleEnumModify,
	"galleryChapterUpdate":  models.RoleEnumModify,
	"galleryChapterDestroy": models.RoleEnumAdmin,

	// performers
	"performerCreate":     models.RoleEnumModify,
	"performerUpdate":     models.RoleEnumModify,
	"performerDestroy":    models.RoleEnumAdmin,
	"performersDestroy":   models.RoleEnumAdmin,
	"bulkPerformerUpdate": models.RoleEnumModify,
	"performerMerge":      models.RoleEnumAdmin,

	// studios
	"studioCreate":     models.RoleEnumModify,
	"studioUpdate":     models.RoleEnumModify,
	"studioDestroy":    models.RoleEnumAdmin,
	"studiosDestroy":   models.RoleEnumAdmin,
	"bulkStudioUpdate": models.RoleEnumModify,

	// movies (deprecated) and groups
	"movieCreate":          models.RoleEnumModify,
	"movieUpdate":          models.RoleEnumModify,
	"movieDestroy":         models.RoleEnumAdmin,
	"moviesDestroy":        models.RoleEnumAdmin,
	"bulkMovieUpdate":      models.RoleEnumModify,
	"groupCreate":          models.RoleEnumModify,
	"groupUpdate":          models.RoleEnumModify,
	"groupDestroy":         models.RoleEnumAdmin,
	"groupsDestroy":        models.RoleEnumAdmin,
	"bulkGroupUpdate":      models.RoleEnumModify,
	"addGroupSubGroups":    models.RoleEnumModify,
	"removeGroupSubGroups": models.RoleEnumModify,
	"reorderSubGroups":     models.RoleEnumModify,

	// tags
	"tagCreate":     models.RoleEnumModify,
	"tagUpdate":     models.RoleEnumModify,
	"tagDestroy":    models.RoleEnumAdmin,
	"tagsDestroy":   models.RoleEnumAdmin,
	"tagsMerge":     models.RoleEnumAdmin,
	"bulkTagUpdate": models.RoleEnumModify,

	// files
	"moveFiles":                 models.RoleEnumAdmin,
	"deleteFiles":               models.RoleEnumAdmin,
	"destroyFiles":              models.RoleEnumAdmin,
	"fileSetFingerprints":       models.RoleEnumAdmin,
	"revealFileInFileManager":   models.RoleEnumAdmin,
	"revealFolderInFileManager": models.RoleEnumAdmin,

	// saved filters
	"saveFilter":         models.RoleEnumModify,
	"destroySavedFilter": models.RoleEnumAdmin,
	"setDefaultFilter":   models.RoleEnumAdmin,

	// configuration
	"configureGeneral":   models.RoleEnumAdmin,
	"configureInterface": models.RoleEnumAdmin,
	"configureDLNA":      models.RoleEnumAdmin,
	"configureScraping":  models.RoleEnumAdmin,
	"configureDefaults":  models.RoleEnumAdmin,
	"configurePlugin":    models.RoleEnumAdmin,
	"configureUI":        models.RoleEnumAdmin,
	"configureUISetting": models.RoleEnumAdmin,
	"generateAPIKey":     models.RoleEnumAdmin,

	// tasks and jobs
	"exportObjects":           models.RoleEnumAdmin,
	"importObjects":           models.RoleEnumAdmin,
	"metadataImport":          models.RoleEnumAdmin,
	"metadataExport":          models.RoleEnumAdmin,
	"metadataScan":            models.RoleEnumAdmin,
	"metadataGenerate":        models.RoleEnumAdmin,
	"metadataAutoTag":         models.RoleEnumAdmin,
	"metadataClean":           models.RoleEnumAdmin,
	"metadataCleanGenerated":  models.RoleEnumAdmin,
	"metadataIdentify":        models.RoleEnumAdmin,
	"migrateHashNaming":       models.RoleEnumAdmin,
	"migrateSceneScreenshots": models.RoleEnumAdmin,
	"migrateBlobs":            models.RoleEnumAdmin,
	"anonymiseDatabase":       models.RoleEnumAdmin,
	"optimiseDatabase":        models.RoleEnumAdmin,
	"backupDatabase":          models.RoleEnumAdmin,
	"stopJob":                 models.RoleEnumAdmin,
	"stopAllJobs":             models.RoleEnumAdmin,

	// scrapers, plugins and packages
	"reloadScrapers":     models.RoleEnumAdmin,
	"setPluginsEnabled":  models.RoleEnumAdmin,
	"runPluginTask":      models.RoleEnumAdmin,
	"runPluginOperation": models.RoleEnumAdmin,
	"reloadPlugins":      models.RoleEnumAdmin,
	"installPackages":    models.RoleEnumAdmin,
	"updatePackages":     models.RoleEnumAdmin,
	"uninstallPackages":  models.RoleEnumAdmin,

	// stash-box
	"submitStashBoxFingerprints":   models.RoleEnumAdmin,
	"submitStashBoxSceneDraft":     models.RoleEnumAdmin,
	"submitStashBoxPerformerDraft": models.RoleEnumAdmin,
	"stashBoxBatchPerformerTag":    models.RoleEnumAdmin,
	"stashBoxBatchStudioTag":       models.RoleEnumAdmin,
	"stashBoxBatchTagTag":          models.RoleEnumAdmin,

	// raw SQL
	"querySQL": models.RoleEnumAdmin,
	"execSQL":  models.RoleEnumAdmin,

	// DLNA
	"enableDLNA":       models.RoleEnumAdmin,
	"disableDLNA":      models.RoleEnumAdmin,
	"addTempDLNAIP":    models.RoleEnumAdmin,
	"removeTempDLNAIP": models.RoleEnumAdmin,

	// users
	"userCreate":         models.RoleEnumAdmin,
	"userUpdate":         models.RoleEnumAdmin,
	"userDestroy":        models.RoleEnumAdmin,
	"changeUserPassword": models.RoleEnumAdmin,
	"changePassword":     models.RoleEnumRead,
}

// Queries and subscriptions that require more than the READ role.
// All other queries and subscriptions are available to every user.
var expectedRestrictedQueryRoles = map[string]models.RoleEnum{
	"users":                       models.RoleEnumAdmin,
	"logs":                        models.RoleEnumAdmin,
	"directory":                   models.RoleEnumAdmin,
	"installedPackages":           models.RoleEnumAdmin,
	"availablePackages":           models.RoleEnumAdmin,
	"validateStashBoxCredentials": models.RoleEnumAdmin,

	"scrapeSingleScene":     models.RoleEnumModify,
	"scrapeMultiScenes":     models.RoleEnumModify,
	"scrapeSingleStudio":    models.RoleEnumModify,
	"scrapeSingleTag":       models.RoleEnumModify,
	"scrapeSinglePerformer": models.RoleEnumModify,
	"scrapeMultiPerformers": models.RoleEnumModify,
	"scrapeSingleGallery":   models.RoleEnumModify,
	"scrapeSingleMovie":     models.RoleEnumModify,
	"scrapeSingleGroup":     models.RoleEnumModify,
	"scrapeSingleImage":     models.RoleEnumModify,
	"scrapeURL":             models.RoleEnumModify,
	"scrapePerformerURL":    models.RoleEnumModify,
	"scrapeSceneURL":        models.RoleEnumModify,
	"scrapeGalleryURL":      models.RoleEnumModify,
	"scrapeImageURL":        models.RoleEnumModify,
	"scrapeMovieURL":        models.RoleEnumModify,
	"scrapeGroupURL":        models.RoleEnumModify,

	// subscriptions
	"loggingSubscribe": models.RoleEnumAdmin,
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

// TestMutationRoles classifies every mutation of the schema by required role.
// It fails when a mutation is not classified, or is classified differently.
func TestMutationRoles(t *testing.T) {
	schema := loadTestSchema(t)
	require.NotNil(t, schema.Mutation)

	inSchema := make(map[string]bool)
	for _, f := range schemaFields(schema.Mutation) {
		inSchema[f.Name] = true

		t.Run(f.Name, func(t *testing.T) {
			expected, classified := expectedMutationRoles[f.Name]
			if !assert.True(t, classified, "mutation %q is not classified: add it to expectedMutationRoles", f.Name) {
				return
			}

			role, hasDirective := fieldRole(f)
			assert.True(t, hasDirective, "mutation %q must have an explicit @hasRole directive", f.Name)
			assert.True(t, role.IsValid(), "mutation %q has an invalid role %q", f.Name, role)

			// the role that is enforced at runtime
			assert.Equal(t, expected, mutationRole(f), "mutation %q has the wrong role", f.Name)
		})
	}

	var stale []string
	for name := range expectedMutationRoles {
		if !inSchema[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	assert.Empty(t, stale, "classified mutations that are not in the schema")
}

// TestQueryRoles ensures that only the expected queries and subscriptions are
// restricted, and that their roles are as expected.
func TestQueryRoles(t *testing.T) {
	schema := loadTestSchema(t)

	seen := make(map[string]bool)
	for _, def := range []*ast.Definition{schema.Query, schema.Subscription} {
		require.NotNil(t, def)
		for _, f := range schemaFields(def) {
			role, restricted := fieldRole(f)
			expected, expectRestricted := expectedRestrictedQueryRoles[f.Name]
			seen[f.Name] = true

			if !expectRestricted {
				assert.False(t, restricted, "%s.%s is unexpectedly restricted to %s", def.Name, f.Name, role)
				continue
			}

			assert.True(t, restricted, "%s.%s must be restricted to %s", def.Name, f.Name, expected)
			assert.Equal(t, expected, role, "%s.%s has the wrong role", def.Name, f.Name)
		}
	}

	for name := range expectedRestrictedQueryRoles {
		assert.True(t, seen[name], "restricted field %q is not in the schema", name)
	}
}

func TestRoleImplication(t *testing.T) {
	tests := []struct {
		userRole models.RoleEnum
		required models.RoleEnum
		allowed  bool
	}{
		{models.RoleEnumRead, models.RoleEnumRead, true},
		{models.RoleEnumRead, models.RoleEnumModify, false},
		{models.RoleEnumRead, models.RoleEnumAdmin, false},
		{models.RoleEnumModify, models.RoleEnumRead, true},
		{models.RoleEnumModify, models.RoleEnumModify, true},
		{models.RoleEnumModify, models.RoleEnumAdmin, false},
		{models.RoleEnumAdmin, models.RoleEnumRead, true},
		{models.RoleEnumAdmin, models.RoleEnumModify, true},
		{models.RoleEnumAdmin, models.RoleEnumAdmin, true},
	}

	for _, tt := range tests {
		ctx := models.SetCurrentUser(context.Background(), models.User{
			Username: "user",
			Roles:    models.Roles{tt.userRole},
		})

		called := false
		_, err := HasRoleDirective(ctx, nil, func(ctx context.Context) (interface{}, error) {
			called = true
			return nil, nil
		}, tt.required)

		assert.Equal(t, tt.allowed, called, "%s user, %s required", tt.userRole, tt.required)
		if tt.allowed {
			assert.NoError(t, err)
		} else {
			assertPermissionDenied(t, err, tt.required)
		}
	}

	// a locked user (no roles) has no access
	ctx := models.SetCurrentUser(context.Background(), models.User{Username: "locked"})
	assert.Error(t, checkRole(ctx, models.RoleEnumRead))

	// without a current user, authentication is not configured
	assert.NoError(t, checkRole(context.Background(), models.RoleEnumAdmin))
}

func TestIsUserOwnerDirective(t *testing.T) {
	withUser := func(name string, role models.RoleEnum) context.Context {
		return models.SetCurrentUser(context.Background(), models.User{Username: name, Roles: models.Roles{role}})
	}

	owner := &models.User{Username: "viewer", Roles: models.Roles{models.RoleEnumRead}}

	tests := []struct {
		name    string
		ctx     context.Context
		obj     interface{}
		allowed bool
	}{
		{"no current user", context.Background(), owner, true},
		{"owner", withUser("viewer", models.RoleEnumRead), owner, true},
		{"other user", withUser("editor", models.RoleEnumModify), owner, false},
		{"admin", withUser("admin", models.RoleEnumAdmin), owner, true},
		{"not a user", withUser("viewer", models.RoleEnumRead), "viewer", false},
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
	assert.True(t, isPermissionDenied(newPermissionDeniedError(models.RoleEnumAdmin)))
	assert.True(t, isPermissionDenied(fmt.Errorf("resolving: %w", newPermissionDeniedError(models.RoleEnumModify))))
	assert.False(t, isPermissionDenied(errors.New("permission denied")))
	assert.False(t, isPermissionDenied(&gqlerror.Error{Message: "other"}))
}

func assertPermissionDenied(t *testing.T, err error, role models.RoleEnum) {
	t.Helper()

	var gqlErr *gqlerror.Error
	if !assert.True(t, errors.As(err, &gqlErr), "expected gqlerror, got %v", err) {
		return
	}
	assert.Equal(t, errCodePermissionDenied, gqlErr.Extensions["code"])
	assert.Equal(t, role.String(), gqlErr.Extensions["requiredRole"])
	assert.Contains(t, gqlErr.Message, "permission denied")
}

// TestMutationRoleMiddleware tests, through the gqlgen executor and the
// generated schema, that mutations without a @hasRole directive are
// restricted to the ADMIN role.
func TestMutationRoleMiddleware(t *testing.T) {
	// a private copy of the schema, which the test changes
	schema := gqlparser.MustLoadSchema(sources...)
	es := NewExecutableSchema(Config{
		Schema:    schema,
		Resolvers: &Resolver{},
		Directives: DirectiveRoot{
			HasRole:     HasRoleDirective,
			IsUserOwner: IsUserOwnerDirective,
		},
	})

	exec := executor.New(es)
	exec.AroundRootFields(mutationRoleMiddleware)
	// stands in for the resolvers: a root field resolves as soon as the role
	// check passes, so the resolvers (and their directives) are not called
	exec.AroundRootFields(func(ctx context.Context, next graphql.RootResolver) graphql.Marshaler {
		return graphql.MarshalBoolean(true)
	})

	const tagCreate = `mutation { tagCreate(input: {name: "tag"}) { id } }`
	const findTags = `query { findTags { count } }`

	// run executes the operation and returns whether its root field was
	// resolved, and the required role of a permission denied error
	run := func(t *testing.T, query string, role *models.RoleEnum) (resolved bool, deniedRole string) {
		t.Helper()

		ctx := graphql.StartOperationTrace(context.Background())
		if role != nil {
			ctx = models.SetCurrentUser(ctx, models.User{Username: "user", Roles: models.Roles{*role}})
		}

		opCtx, errs := exec.CreateOperationContext(ctx, &graphql.RawParams{Query: query})
		require.Empty(t, errs)

		handler, ctx := exec.DispatchOperation(ctx, opCtx)
		resp := handler(ctx)
		require.NotNil(t, resp)

		if len(resp.Errors) > 0 {
			require.Len(t, resp.Errors, 1)
			require.Equal(t, errCodePermissionDenied, resp.Errors[0].Extensions["code"], resp.Errors[0].Message)
			deniedRole, _ = resp.Errors[0].Extensions["requiredRole"].(string)
		}

		return strings.Contains(string(resp.Data), "true"), deniedRole
	}

	read := models.RoleEnumRead
	modify := models.RoleEnumModify
	admin := models.RoleEnumAdmin

	type testCase struct {
		name       string
		query      string
		role       *models.RoleEnum
		resolved   bool
		deniedRole models.RoleEnum
	}

	runTests := func(t *testing.T, tests []testCase) {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				resolved, deniedRole := run(t, tt.query, tt.role)
				assert.Equal(t, tt.resolved, resolved, "resolved")
				assert.Equal(t, tt.deniedRole.String(), deniedRole, "required role")
			})
		}
	}

	// tagCreate has the MODIFY role in the schema
	t.Run("annotated", func(t *testing.T) {
		runTests(t, []testCase{
			{"mutation as viewer", tagCreate, &read, false, models.RoleEnumModify},
			{"mutation as editor", tagCreate, &modify, true, ""},
			{"mutation as admin", tagCreate, &admin, true, ""},
			{"query as viewer", findTags, &read, true, ""},
		})
	})

	// remove the @hasRole directive of tagCreate, to simulate a mutation
	// added without one
	def := schema.Mutation.Fields.ForName("tagCreate")
	require.NotNil(t, def)
	def.Directives = nil

	t.Run("unannotated", func(t *testing.T) {
		runTests(t, []testCase{
			{"mutation as viewer", tagCreate, &read, false, models.RoleEnumAdmin},
			{"mutation as editor", tagCreate, &modify, false, models.RoleEnumAdmin},
			{"mutation as admin", tagCreate, &admin, true, ""},
			{"mutation without authentication", tagCreate, nil, true, ""},
		})
	})
}
