package config

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnitOf(t *testing.T) {
	tests := []struct {
		section PersonalSection
		key     string
		unit    string
		parent  bool
	}{
		{PersonalSectionInterface, "language", "language", false},
		{PersonalSectionInterface, "imageLightbox.slideshowDelay", "imageLightbox.slideshowDelay", false},
		{PersonalSectionInterface, "imageLightbox", "imageLightbox", true},
		{PersonalSectionInterface, "disableDropdownCreate.movie", "disableDropdownCreate.movie", false},
		{PersonalSectionUI, "tableColumns.scenes", "tableColumns.scenes", false},
		{PersonalSectionUI, "tableColumns.scenes.extra", "tableColumns.scenes", false},
		{PersonalSectionUI, "tableColumns", "tableColumns", true},
		{PersonalSectionUI, "taggerConfig.blacklist", "taggerConfig", false},
		{PersonalSectionUI, "ratingSystemOptions", "ratingSystemOptions", false},
	}

	for _, tt := range tests {
		unit, parent := UnitOf(tt.section, tt.key)
		assert.Equal(t, tt.unit, unit, "%s %s", tt.section, tt.key)
		assert.Equal(t, tt.parent, parent, "%s %s", tt.section, tt.key)
	}
}

func TestIsSharedAndListSetting(t *testing.T) {
	assert.True(t, IsShared(PersonalSectionInterface, "javascript"))
	assert.True(t, IsShared(PersonalSectionInterface, "handyKey"))
	assert.False(t, IsShared(PersonalSectionInterface, "language"))
	assert.True(t, IsShared(PersonalSectionUI, "title"))
	assert.True(t, IsShared(PersonalSectionUI, "taskDefaults.scan"))
	assert.False(t, IsShared(PersonalSectionUI, "pluginKey"))

	assert.True(t, IsListSetting(PersonalSettingKey{PersonalSectionUI, "tableColumns.scenes"}))
	assert.True(t, IsListSetting(PersonalSettingKey{PersonalSectionUI, "frontPageContent"}))
	assert.True(t, IsListSetting(PersonalSettingKey{PersonalSectionUI, "taggerConfig"}))
	assert.False(t, IsListSetting(PersonalSettingKey{PersonalSectionUI, "ratingSystemOptions"}))
	assert.False(t, IsListSetting(PersonalSettingKey{PersonalSectionInterface, "language"}))
}

func TestApplyUnits(t *testing.T) {
	defaults := map[string]interface{}{
		"tableColumns": map[string]interface{}{
			"scenes": []interface{}{"title"},
			"images": []interface{}{"title", "rating"},
		},
		"ratingSystemOptions": map[string]interface{}{"type": "stars", "starPrecision": "full"},
		"title":               "Shared",
		"previewVolume":       25,
	}

	personal := map[string]interface{}{
		"tableColumns": map[string]interface{}{
			"scenes": []interface{}{"title", "date"},
		},
		"ratingSystemOptions": map[string]interface{}{"type": "decimal"},
		// shared keys of a personal map are ignored
		"title": "Mine",
	}

	got := ApplyUnits(PersonalSectionUI, defaults, personal)

	assert.Equal(t, map[string]interface{}{
		"tableColumns": map[string]interface{}{
			"scenes": []interface{}{"title", "date"},
			"images": []interface{}{"title", "rating"},
		},
		// replaced as a whole
		"ratingSystemOptions": map[string]interface{}{"type": "decimal"},
		"title":               "Shared",
		"previewVolume":       25,
	}, got)

	// the defaults are not changed
	assert.Equal(t, []interface{}{"title"}, defaults["tableColumns"].(map[string]interface{})["scenes"])

	// changing the result does not change the personal map
	got["tableColumns"].(map[string]interface{})["scenes"].([]interface{})[0] = "changed"
	assert.Equal(t, "title", personal["tableColumns"].(map[string]interface{})["scenes"].([]interface{})[0])
}

func TestApplyUnits_Interface(t *testing.T) {
	defaults := map[string]interface{}{
		"language": "en-GB",
		"imageLightbox": map[string]interface{}{
			"slideshowDelay": 5,
			"scaleUp":        true,
		},
		"css": "body {}",
	}

	personal := map[string]interface{}{
		"language":      "hu-HU",
		"imageLightbox": map[string]interface{}{"slideshowDelay": 3},
		"css":           "evil {}",
	}

	got := ApplyUnits(PersonalSectionInterface, defaults, personal)
	assert.Equal(t, map[string]interface{}{
		"language": "hu-HU",
		"imageLightbox": map[string]interface{}{
			"slideshowDelay": 3,
			"scaleUp":        true,
		},
		"css": "body {}",
	}, got)
}

func TestSetUnits(t *testing.T) {
	personal := map[string]interface{}{
		"tableColumns": map[string]interface{}{
			"images": []interface{}{"title"},
		},
		"vrTag": "VR",
	}

	require.NoError(t, SetUnits(PersonalSectionUI, personal, map[string]interface{}{
		// a parent sets its children one by one
		"tableColumns": map[string]interface{}{"scenes": []interface{}{"date"}},
		// nil removes the unit
		"vrTag":         nil,
		"previewVolume": 50,
	}))

	assert.Equal(t, map[string]interface{}{
		"tableColumns": map[string]interface{}{
			"images": []interface{}{"title"},
			"scenes": []interface{}{"date"},
		},
		"previewVolume": 50,
	}, personal)

	// nil for a parent removes all of its units
	require.NoError(t, SetUnits(PersonalSectionUI, personal, map[string]interface{}{"tableColumns": nil}))
	assert.Equal(t, map[string]interface{}{"previewVolume": 50}, personal)

	assert.Error(t, SetUnits(PersonalSectionUI, personal, map[string]interface{}{"lastNoteSeen": 1}))
	assert.Error(t, SetUnits(PersonalSectionInterface, personal, map[string]interface{}{"javascript": "x"}))
	assert.Error(t, SetUnits(PersonalSectionUI, personal, map[string]interface{}{"tableColumns": "x"}))
	assert.Equal(t, map[string]interface{}{"previewVolume": 50}, personal, "an error changes nothing")
}

func TestUnitsAndDeleteUnit(t *testing.T) {
	m := map[string]interface{}{
		"tableColumns": map[string]interface{}{
			"scenes": []interface{}{"date"},
		},
		"advancedMode": true,
		"title":        "ignored",
	}

	units := Units(PersonalSectionUI, m)
	require.Len(t, units, 2)
	assert.Equal(t, "advancedMode", units[0].Key)
	assert.Equal(t, "tableColumns.scenes", units[1].Key)
	assert.Equal(t, PersonalSectionUI, units[1].Section)

	DeleteUnit(m, "tableColumns.scenes")
	_, found := m["tableColumns"]
	assert.False(t, found, "an emptied parent is removed")
}

func TestUserStore_PersonalSettings(t *testing.T) {
	ctx := context.Background()
	s, fn := newTestUserStore(t, "")

	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: models.RoleIDAdmin}, "adminpw"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: models.RoleIDViewer}, "viewerpw"))

	assert.True(t, s.IsPersonalUser("viewer"))
	assert.False(t, s.IsPersonalUser("nobody"))

	require.NoError(t, s.UpdatePersonalSettings("viewer", func(iface, ui map[string]interface{}) error {
		if err := SetUnits(PersonalSectionInterface, iface, map[string]interface{}{
			"language":      "hu-HU",
			"imageLightbox": map[string]interface{}{"slideshowDelay": 3},
		}); err != nil {
			return err
		}

		return SetUnits(PersonalSectionUI, ui, map[string]interface{}{
			"tableColumns":        map[string]interface{}{"scenes": []interface{}{"title", "date"}},
			"ratingSystemOptions": map[string]interface{}{"type": "decimal"},
		})
	}))

	check := func(s *UserStore, username string) {
		t.Helper()

		iface, ui, err := s.PersonalSettings(username)
		require.NoError(t, err)
		assert.Equal(t, "hu-HU", iface["language"])
		assert.EqualValues(t, 3, iface["imageLightbox"].(map[string]interface{})["slideshowDelay"])
		assert.Equal(t, []interface{}{"title", "date"}, ui["tableColumns"].(map[string]interface{})["scenes"])
		assert.Equal(t, "decimal", ui["ratingSystemOptions"].(map[string]interface{})["type"])
	}

	check(s, "viewer")

	// the settings are written to the config file, with their case
	check(loadTestUserStore(t, fn), "viewer")

	// the other users have none
	iface, ui, err := s.PersonalSettings("admin")
	require.NoError(t, err)
	assert.Empty(t, iface)
	assert.Empty(t, ui)

	// changes of the user keep the settings
	require.NoError(t, s.ChangeUserPassword(ctx, "viewer", "newpassword"))
	require.NoError(t, s.ReplaceUser(ctx, "viewer", models.User{Username: "renamed", Role: models.RoleIDEditor}))
	check(s, "renamed")
	check(loadTestUserStore(t, fn), "renamed")

	// a failing update changes nothing
	assert.Error(t, s.UpdatePersonalSettings("renamed", func(iface, ui map[string]interface{}) error {
		iface["language"] = "de-DE"
		return assert.AnError
	}))
	check(s, "renamed")

	// deleting the user deletes the settings
	require.NoError(t, s.DeleteUser(ctx, "renamed"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "renamed", Role: models.RoleIDViewer}, "viewerpw"))
	iface, ui, err = s.PersonalSettings("renamed")
	require.NoError(t, err)
	assert.Empty(t, iface)
	assert.Empty(t, ui)

	_, _, err = s.PersonalSettings("nobody")
	assert.ErrorIs(t, err, ErrNotPersonal)
	assert.ErrorIs(t, s.UpdatePersonalSettings("nobody", func(iface, ui map[string]interface{}) error { return nil }), ErrNotPersonal)
}

func TestUserStore_PersonalSettingsLegacyUser(t *testing.T) {
	s, _ := newTestUserStore(t, "username: legacy\npassword: "+mustHash(t, "legacypw")+"\n")

	assert.False(t, s.IsPersonalUser("legacy"), "the legacy user shares the settings")
	_, _, err := s.PersonalSettings("legacy")
	assert.ErrorIs(t, err, ErrNotPersonal)
	assert.ErrorIs(t, s.UpdatePersonalSettings("legacy", func(iface, ui map[string]interface{}) error { return nil }), ErrNotPersonal)
}

func TestUserStore_CopyPersonalSettings(t *testing.T) {
	ctx := context.Background()
	s, fn := newTestUserStore(t, "")

	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: models.RoleIDAdmin}, "adminpw"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "anna", Role: models.RoleIDViewer}, "annapass"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "bela", Role: models.RoleIDViewer}, "belapass"))

	require.NoError(t, s.UpdatePersonalSettings("anna", func(iface, ui map[string]interface{}) error {
		iface["language"] = "hu-HU"
		ui["tableColumns"] = map[string]interface{}{"scenes": []interface{}{"title"}}
		return nil
	}))
	require.NoError(t, s.UpdatePersonalSettings("bela", func(iface, ui map[string]interface{}) error {
		iface["menuItems"] = []interface{}{"scenes"}
		return nil
	}))

	require.NoError(t, s.CopyPersonalSettings("anna", "bela"))

	iface, ui, err := loadTestUserStore(t, fn).PersonalSettings("bela")
	require.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"language": "hu-HU"}, iface, "the settings are replaced")
	assert.Equal(t, map[string]interface{}{"tableColumns": map[string]interface{}{"scenes": []interface{}{"title"}}}, ui)

	// the copies are independent
	require.NoError(t, s.UpdatePersonalSettings("anna", func(iface, ui map[string]interface{}) error {
		iface["language"] = "de-DE"
		return nil
	}))
	iface, _, err = s.PersonalSettings("bela")
	require.NoError(t, err)
	assert.Equal(t, "hu-HU", iface["language"])

	assert.Error(t, s.CopyPersonalSettings("nobody", "bela"))
}

func TestUserStore_PersonalSettingsSizeLimit(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestUserStore(t, "")
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: models.RoleIDAdmin}, "adminpw"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: models.RoleIDViewer}, "viewerpw"))

	big := strings.Repeat("x", MaxPersonalSettingsSize)
	setUI := func(key string, value interface{}) error {
		return s.UpdatePersonalSettings("viewer", func(_, ui map[string]interface{}) error {
			ui[key] = value
			return nil
		})
	}

	// a change that makes the settings too large is refused, and changes
	// nothing
	assert.ErrorIs(t, setUI("plugin", big), ErrPersonalSettingsTooLarge)
	_, ui, err := s.PersonalSettings("viewer")
	require.NoError(t, err)
	assert.Empty(t, ui)

	require.NoError(t, setUI("plugin", "small"))

	// settings that are already too large can only shrink
	require.NoError(t, s.modifyUsers(func(users []StoredUser) ([]StoredUser, error) {
		users[s.findUser(users, "viewer")].UI = map[string]interface{}{"a": big, "b": big}
		return users, nil
	}))
	assert.ErrorIs(t, setUI("c", "more"), ErrPersonalSettingsTooLarge)
	assert.NoError(t, s.UpdatePersonalSettings("viewer", func(_, ui map[string]interface{}) error {
		delete(ui, "a")
		return nil
	}))
}

// Every request reads the users: the reads must not copy the personal
// settings of every user each time.
func TestUserStore_RequestReadsDoNotCopyPersonalSettings(t *testing.T) {
	ctx := context.Background()
	s, _ := newTestUserStore(t, "")
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: models.RoleIDAdmin}, "adminpw"))
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "viewer", Role: models.RoleIDViewer}, "viewerpw"))
	require.NoError(t, s.UpdatePersonalSettings("admin", func(_, ui map[string]interface{}) error {
		for i := 0; i < 1000; i++ {
			ui[fmt.Sprintf("key%d", i)] = []interface{}{"a", "b"}
		}
		return nil
	}))

	allocs := testing.AllocsPerRun(10, func() {
		_ = s.LoginRequired(ctx)
		_, _ = s.GetUser(ctx, "viewer")
		_, _ = s.GetSessionToken(ctx, "viewer")
	})
	assert.Less(t, allocs, 100.0)

	// a change is read at once
	require.NoError(t, s.ReplaceUser(ctx, "viewer", models.User{Username: "viewer", Role: models.RoleIDEditor}))
	u, err := s.GetUser(ctx, "viewer")
	require.NoError(t, err)
	assert.Equal(t, models.RoleIDEditor, u.Role)
	require.NoError(t, s.DeleteUser(ctx, "viewer"))
	u, err = s.GetUser(ctx, "viewer")
	require.NoError(t, err)
	assert.Nil(t, u)
}

func TestUserStore_UpdatePersonalSettingsUnchanged(t *testing.T) {
	ctx := context.Background()
	s, fn := newTestUserStore(t, "")
	require.NoError(t, s.CreateUser(ctx, models.User{Username: "admin", Role: models.RoleIDAdmin}, "adminpw"))

	set := func(iface, _ map[string]interface{}) error {
		iface["language"] = "hu-HU"
		return nil
	}
	require.NoError(t, s.UpdatePersonalSettings("admin", set))

	// an update that changes nothing does not write the config file
	require.NoError(t, os.Remove(fn))
	require.NoError(t, s.UpdatePersonalSettings("admin", set))
	assert.NoFileExists(t, fn)
}
