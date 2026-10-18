package api

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPersonalSettingsConfig initialises the config instance with a config
// file in a temporary directory, the users admin, viewer and lister, and a
// shared UI configuration.
func newPersonalSettingsConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg := config.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))

	ctx := context.Background()
	require.NoError(t, cfg.UserStore.CreateUser(ctx, models.User{Username: "admin", Role: models.RoleIDAdmin}, "adminpw"))
	require.NoError(t, cfg.UserStore.CreateUser(ctx, models.User{Username: "viewer", Role: models.RoleIDViewer}, "viewerpw"))
	require.NoError(t, cfg.UserStore.CreateUser(ctx, models.User{Username: "lister", Role: models.RoleIDViewer}, "listerpw"))

	cfg.SetUIConfiguration(map[string]interface{}{
		"tableColumns": map[string]interface{}{
			"scenes": []interface{}{"title"},
			"images": []interface{}{"rating"},
		},
		"taggerConfig": map[string]interface{}{"mode": "auto"},
		"title":        "Shared",
	})

	return cfg
}

func personalCtx(username string, roleID string, permissions ...models.Permission) context.Context {
	return models.SetCurrentUser(context.Background(), models.User{
		Username:    username,
		Role:        roleID,
		Permissions: permissions,
	})
}

func effectiveLanguage(t *testing.T, ctx context.Context) string {
	t.Helper()

	c, err := (&queryResolver{&Resolver{}}).Configuration(ctx)
	require.NoError(t, err)
	require.NotNil(t, c.Interface.Language)
	return *c.Interface.Language
}

func effectiveUI(t *testing.T, ctx context.Context) map[string]interface{} {
	t.Helper()

	c, err := (&queryResolver{&Resolver{}}).Configuration(ctx)
	require.NoError(t, err)
	return c.UI
}

func strPtr(s string) *string {
	return &s
}

func TestConfigureInterfacePersonal(t *testing.T) {
	cfg := newPersonalSettingsConfig(t)
	r := &mutationResolver{&Resolver{}}
	admin := personalCtx("admin", models.RoleIDAdmin)
	viewer := personalCtx("viewer", models.RoleIDViewer)
	sharedLanguage := cfg.GetLanguage()

	got, err := r.ConfigureInterface(viewer, ConfigInterfaceInput{Language: strPtr("hu-HU")})
	require.NoError(t, err)
	assert.Equal(t, "hu-HU", *got.Language, "the result is the effective configuration")

	assert.Equal(t, "hu-HU", effectiveLanguage(t, viewer))
	assert.Equal(t, sharedLanguage, effectiveLanguage(t, admin), "other users keep the default")
	assert.Equal(t, sharedLanguage, cfg.GetLanguage(), "the default is unchanged")

	// a shared field requires the Admin role, even when turning it off
	cssEnabled := false
	_, err = r.ConfigureInterface(viewer, ConfigInterfaceInput{Language: strPtr("de-DE"), CSSEnabled: &cssEnabled})
	assert.True(t, isPermissionDenied(err), "shared field: %v", err)
	assert.Equal(t, "hu-HU", effectiveLanguage(t, viewer), "a denied request changes nothing")

	// an admin changes the shared fields for everyone, and the others for
	// themselves
	cssEnabled = true
	_, err = r.ConfigureInterface(admin, ConfigInterfaceInput{Language: strPtr("de-DE"), CSSEnabled: &cssEnabled})
	require.NoError(t, err)
	assert.True(t, cfg.GetCSSEnabled())
	assert.Equal(t, "de-DE", effectiveLanguage(t, admin))
	assert.Equal(t, sharedLanguage, cfg.GetLanguage())
	assert.Equal(t, "hu-HU", effectiveLanguage(t, viewer))

	// the fields of the nested objects are inherited one by one
	delay := 3
	got, err = r.ConfigureInterface(viewer, ConfigInterfaceInput{ImageLightbox: &ConfigImageLightboxInput{SlideshowDelay: &delay}})
	require.NoError(t, err)
	assert.Equal(t, 3, *got.ImageLightbox.SlideshowDelay)
	assert.Equal(t, cfg.GetImageLightboxOptions().ScrollAttemptsBeforeChange, got.ImageLightbox.ScrollAttemptsBeforeChange)
}

func TestConfigureInterfacePersonalRedactsHandyKey(t *testing.T) {
	cfg := newPersonalSettingsConfig(t)
	cfg.SetString(config.HandyKey, "handy-secret")
	r := &mutationResolver{&Resolver{}}

	// the result of a personal change is redacted as the configuration query
	// is: only admins get the key
	got, err := r.ConfigureInterface(personalCtx("viewer", models.RoleIDViewer), ConfigInterfaceInput{Language: strPtr("hu-HU")})
	require.NoError(t, err)
	require.NotNil(t, got.HandyKey)
	assert.Equal(t, "", *got.HandyKey)

	got, err = r.ConfigureInterface(personalCtx("admin", models.RoleIDAdmin), ConfigInterfaceInput{Language: strPtr("hu-HU")})
	require.NoError(t, err)
	require.NotNil(t, got.HandyKey)
	assert.Equal(t, "handy-secret", *got.HandyKey)
}

func TestConfigureUIPersonal(t *testing.T) {
	cfg := newPersonalSettingsConfig(t)
	r := &mutationResolver{&Resolver{}}
	admin := personalCtx("admin", models.RoleIDAdmin)
	viewer := personalCtx("viewer", models.RoleIDViewer)

	got, err := r.ConfigureUISetting(viewer, "tableColumns.scenes", []interface{}{"date"})
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{
		"scenes": []interface{}{"date"},
		"images": []interface{}{"rating"},
	}, got["tableColumns"], "the other tables follow the defaults")
	assert.Equal(t, []interface{}{"title"}, effectiveUI(t, admin)["tableColumns"].(map[string]interface{})["scenes"])

	// a key below its unit changes a copy of the unit
	_, err = r.ConfigureUISetting(viewer, "taggerConfig.blacklist", []interface{}{"x"})
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"mode": "auto", "blacklist": []interface{}{"x"}}, effectiveUI(t, viewer)["taggerConfig"])
	assert.Equal(t, map[string]interface{}{"mode": "auto"}, cfg.GetUIConfiguration()["taggerConfig"])

	// null resets the unit
	_, err = r.ConfigureUISetting(viewer, "tableColumns.scenes", nil)
	require.NoError(t, err)
	assert.Equal(t, []interface{}{"title"}, effectiveUI(t, viewer)["tableColumns"].(map[string]interface{})["scenes"])

	got, err = r.ConfigureUI(viewer, nil, map[string]interface{}{"previewVolume": 50})
	require.NoError(t, err)
	assert.Equal(t, 50, got["previewVolume"])
	_, found := cfg.GetUIConfiguration()["previewVolume"]
	assert.False(t, found)

	// the shared keys and replacing the configuration require the Admin role
	_, err = r.ConfigureUI(viewer, nil, map[string]interface{}{"title": "Mine"})
	assert.True(t, isPermissionDenied(err), "partial title: %v", err)
	_, err = r.ConfigureUISetting(viewer, "lastNoteSeen", 1)
	assert.True(t, isPermissionDenied(err), "lastNoteSeen: %v", err)
	_, err = r.ConfigureUI(viewer, map[string]interface{}{"previewVolume": 10}, nil)
	assert.True(t, isPermissionDenied(err), "input: %v", err)
	assert.Equal(t, "Shared", cfg.GetUIConfiguration()["title"])

	_, err = r.ConfigureUISetting(admin, "title", "Home")
	require.NoError(t, err)
	assert.Equal(t, "Home", cfg.GetUIConfiguration()["title"])
	assert.Equal(t, "Home", effectiveUI(t, viewer)["title"])
}

func TestPersonalSettingsWithoutUsers(t *testing.T) {
	cfg := config.InitializeEmpty()
	cfg.SetConfigFile(filepath.Join(t.TempDir(), "config.yml"))
	r := &mutationResolver{&Resolver{}}
	q := &queryResolver{&Resolver{}}
	ctx := context.Background()

	_, err := r.ConfigureInterface(ctx, ConfigInterfaceInput{Language: strPtr("hu-HU")})
	require.NoError(t, err)
	assert.Equal(t, "hu-HU", cfg.GetLanguage(), "the shared configuration is changed")

	_, err = r.ConfigureUISetting(ctx, "tableColumns.scenes", []interface{}{"date"})
	require.NoError(t, err)
	assert.Equal(t, []interface{}{"date"}, cfg.GetUIConfiguration()["tableColumns"].(map[string]interface{})["scenes"])

	settings, err := q.PersonalSettings(ctx)
	require.NoError(t, err)
	assert.Empty(t, settings)

	ok, err := r.ResetPersonalSettings(ctx, nil)
	assert.NoError(t, err)
	assert.True(t, ok)
	ok, err = r.PublishPersonalSettings(ctx, []*PersonalSettingInput{{Section: PersonalSettingSectionInterface, Key: "language"}})
	assert.NoError(t, err)
	assert.True(t, ok)
}

// TestSharedUIFallbackRequiresAdmin checks that a signed-in user who is not in
// the users list, and so has no personal settings, cannot change the shared UI
// configuration without the Admin role.
func TestSharedUIFallbackRequiresAdmin(t *testing.T) {
	cfg := newPersonalSettingsConfig(t)
	r := &mutationResolver{&Resolver{}}
	unlisted := personalCtx("unlisted", models.RoleIDViewer, models.PermissionManageSharedListSettings)

	_, err := r.ConfigureUISetting(unlisted, "title", "Changed")
	assert.True(t, isPermissionDenied(err), "setting: %v", err)
	_, err = r.ConfigureUI(unlisted, nil, map[string]interface{}{"title": "Changed"})
	assert.True(t, isPermissionDenied(err), "partial: %v", err)
	assert.Equal(t, "Shared", cfg.GetUIConfiguration()["title"])
}

func TestRemovePublished(t *testing.T) {
	personal := map[string]interface{}{
		"language":  "de-DE",
		"menuItems": []interface{}{"scenes"},
	}

	removePublished(personal, map[string]interface{}{
		"language":  "hu-HU",
		"menuItems": []interface{}{"scenes"},
	})

	// a setting changed since it was read for publishing stays personal
	assert.Equal(t, map[string]interface{}{"language": "de-DE"}, personal)
}

func TestPersonalSettingsAndReset(t *testing.T) {
	newPersonalSettingsConfig(t)
	r := &mutationResolver{&Resolver{}}
	q := &queryResolver{&Resolver{}}
	viewer := personalCtx("viewer", models.RoleIDViewer)

	_, err := r.ConfigureInterface(viewer, ConfigInterfaceInput{Language: strPtr("hu-HU")})
	require.NoError(t, err)
	_, err = r.ConfigureUISetting(viewer, "tableColumns.scenes", []interface{}{"date"})
	require.NoError(t, err)

	settings, err := q.PersonalSettings(viewer)
	require.NoError(t, err)
	require.Len(t, settings, 2)
	assert.Equal(t, PersonalSettingSectionInterface, settings[0].Section)
	assert.Equal(t, "language", settings[0].Key)
	assert.Equal(t, "hu-HU", settings[0].Value)
	assert.Equal(t, PersonalSettingSectionUI, settings[1].Section)
	assert.Equal(t, "tableColumns.scenes", settings[1].Key)
	assert.Equal(t, []interface{}{"title"}, settings[1].DefaultValue)

	ok, err := r.ResetPersonalSettings(viewer, []*PersonalSettingInput{{Section: PersonalSettingSectionUI, Key: "tableColumns.scenes"}})
	require.NoError(t, err)
	assert.True(t, ok)
	settings, err = q.PersonalSettings(viewer)
	require.NoError(t, err)
	require.Len(t, settings, 1)
	assert.Equal(t, "language", settings[0].Key)

	_, err = r.ResetPersonalSettings(viewer, nil)
	require.NoError(t, err)
	settings, err = q.PersonalSettings(viewer)
	require.NoError(t, err)
	assert.Empty(t, settings)
}

func TestPublishPersonalSettings(t *testing.T) {
	cfg := newPersonalSettingsConfig(t)
	r := &mutationResolver{&Resolver{}}
	q := &queryResolver{&Resolver{}}
	admin := personalCtx("admin", models.RoleIDAdmin)
	viewer := personalCtx("viewer", models.RoleIDViewer)
	lister := personalCtx("lister", models.RoleIDViewer, models.PermissionManageSharedListSettings)

	_, err := r.ConfigureInterface(admin, ConfigInterfaceInput{Language: strPtr("de-DE")})
	require.NoError(t, err)
	_, err = r.ConfigureUISetting(admin, "tableColumns.scenes", []interface{}{"date"})
	require.NoError(t, err)

	ok, err := r.PublishPersonalSettings(admin, []*PersonalSettingInput{
		{Section: PersonalSettingSectionInterface, Key: "language"},
		{Section: PersonalSettingSectionUI, Key: "tableColumns.scenes"},
	})
	require.NoError(t, err)
	assert.True(t, ok)

	assert.Equal(t, "de-DE", cfg.GetLanguage())
	assert.Equal(t, "de-DE", effectiveLanguage(t, viewer), "the users without a value get the new default")
	assert.Equal(t, []interface{}{"date"}, cfg.GetUIConfiguration()["tableColumns"].(map[string]interface{})["scenes"])
	settings, err := q.PersonalSettings(admin)
	require.NoError(t, err)
	assert.Empty(t, settings, "the published settings are the defaults now")

	// a list setting with the list settings permission
	_, err = r.ConfigureUISetting(lister, "tableColumns.images", []interface{}{"title"})
	require.NoError(t, err)
	_, err = r.ConfigureUISetting(lister, "ratingSystemOptions", map[string]interface{}{"type": "decimal"})
	require.NoError(t, err)

	// another setting requires the Admin role, and nothing is published
	_, err = r.PublishPersonalSettings(lister, []*PersonalSettingInput{
		{Section: PersonalSettingSectionUI, Key: "tableColumns.images"},
		{Section: PersonalSettingSectionUI, Key: "ratingSystemOptions"},
	})
	assert.True(t, isPermissionDenied(err), "ratingSystemOptions: %v", err)
	assert.Equal(t, []interface{}{"rating"}, cfg.GetUIConfiguration()["tableColumns"].(map[string]interface{})["images"])

	_, err = r.PublishPersonalSettings(lister, []*PersonalSettingInput{{Section: PersonalSettingSectionUI, Key: "tableColumns.images"}})
	require.NoError(t, err)
	assert.Equal(t, []interface{}{"title"}, cfg.GetUIConfiguration()["tableColumns"].(map[string]interface{})["images"])

	// without the permission
	_, err = r.ConfigureUISetting(viewer, "tableColumns.images", []interface{}{"date"})
	require.NoError(t, err)
	_, err = r.PublishPersonalSettings(viewer, []*PersonalSettingInput{{Section: PersonalSettingSectionUI, Key: "tableColumns.images"}})
	assert.True(t, isPermissionDenied(err), "viewer: %v", err)

	// a setting the user did not change
	_, err = r.PublishPersonalSettings(admin, []*PersonalSettingInput{{Section: PersonalSettingSectionUI, Key: "tableColumns.images"}})
	assert.Error(t, err)
	assert.False(t, isPermissionDenied(err))
}

type fakeSFWConfig bool

func (c fakeSFWConfig) GetSFWContentMode() bool {
	return bool(c)
}

func TestPerformerRoutesSFWContentModePersonal(t *testing.T) {
	newPersonalSettingsConfig(t)
	r := &mutationResolver{&Resolver{}}
	rs := performerRoutes{sfwConfig: fakeSFWConfig(false)}
	viewer := personalCtx("viewer", models.RoleIDViewer)

	sfw := true
	_, err := r.ConfigureInterface(viewer, ConfigInterfaceInput{SfwContentMode: &sfw})
	require.NoError(t, err)

	assert.True(t, rs.sfwContentMode(viewer), "own")
	assert.False(t, rs.sfwContentMode(personalCtx("admin", models.RoleIDAdmin)), "shared")
	assert.False(t, rs.sfwContentMode(context.Background()), "no user")
}

func TestFindDefaultFilterPersonal(t *testing.T) {
	newPersonalSettingsConfig(t)
	r := &mutationResolver{&Resolver{}}
	q := &queryResolver{&Resolver{}}
	viewer := personalCtx("viewer", models.RoleIDViewer)

	_, err := r.ConfigureUISetting(viewer, "defaultFilters.scenes", map[string]interface{}{"name": "mine", "mode": "SCENES"})
	require.NoError(t, err)

	got, err := q.FindDefaultFilter(viewer, models.FilterModeScenes)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "mine", got.Name)

	got, err = q.FindDefaultFilter(personalCtx("admin", models.RoleIDAdmin), models.FilterModeScenes)
	require.NoError(t, err)
	assert.Nil(t, got)
}
