package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/stashapp/stash/pkg/user"
	"github.com/stashapp/stash/pkg/utils"
)

// PersonalSection is the part of the configuration that a personal setting
// belongs to.
type PersonalSection string

const (
	// PersonalSectionInterface is the interface configuration. Its keys are
	// the GraphQL names of the fields of ConfigInterfaceInput.
	PersonalSectionInterface PersonalSection = "INTERFACE"
	// PersonalSectionUI is the UI configuration map.
	PersonalSectionUI PersonalSection = "UI"
)

// PersonalSettingKey identifies a unit: a setting that is inherited from the
// defaults, reset and published as a whole.
type PersonalSettingKey struct {
	Section PersonalSection
	Key     string
}

// PersonalSettingValue is a unit with the value that the user set.
type PersonalSettingValue struct {
	PersonalSettingKey
	Value interface{}
}

// ErrNotPersonal is returned for a user who is not in the users list. Without
// a users list, every setting is shared.
var ErrNotPersonal = errors.New("the user has no personal settings")

// MaxPersonalSettingsSize is the largest size of the personal settings of a
// user, in bytes of JSON. The settings are kept in the config file.
const MaxPersonalSettingsSize = 256 * 1024

var ErrPersonalSettingsTooLarge = fmt.Errorf("the personal settings must not be larger than %d KB", MaxPersonalSettingsSize/1024)

var (
	// sharedInterfaceFields are shared by every user, and changed by admins:
	// custom code runs in the browser of every user, the Handy key is a
	// secret, and the others concern the server.
	sharedInterfaceFields = []string{
		"css", "cssEnabled",
		"javascript", "javascriptEnabled",
		"customLocales", "customLocalesEnabled",
		"disableCustomizations",
		"handyKey", "funscriptOffset", "useStashHostedFunscript",
		"noBrowser", "notificationsEnabled",
	}

	// nestedInterfaceFields are objects whose fields are inherited one by one
	nestedInterfaceFields = []string{"imageLightbox", "disableDropdownCreate"}

	// sharedUIKeys are shared by every user, and changed by admins
	sharedUIKeys = []string{"title", "taskDefaults", "lastNoteSeen"}

	// perListUIKeys map the name of a list or table to its setting. The
	// setting of each list is inherited separately.
	perListUIKeys = []string{"defaultFilters", "tableColumns", "pinnedFilters", "pinnedSortBy"}

	// listSettingUIKeys are the list settings, which the holders of the
	// MANAGE_SHARED_LIST_SETTINGS permission publish
	listSettingUIKeys = []string{"defaultFilters", "tableColumns", "pinnedFilters", "pinnedSortBy", "frontPageContent", "taggerConfig"}
)

func firstSegment(key string) string {
	first, _, _ := strings.Cut(key, ".")
	return first
}

// IsShared returns true if the key belongs to a setting that every user
// shares, which only admins change.
func IsShared(section PersonalSection, key string) bool {
	first := firstSegment(key)
	if section == PersonalSectionInterface {
		return slices.Contains(sharedInterfaceFields, first)
	}

	return slices.Contains(sharedUIKeys, first)
}

// IsListSetting returns true if the unit is a list setting.
func IsListSetting(k PersonalSettingKey) bool {
	return k.Section == PersonalSectionUI && slices.Contains(listSettingUIKeys, firstSegment(k.Key))
}

// unitDepth returns the number of key segments of the units below the first
// segment.
func unitDepth(section PersonalSection, first string) int {
	switch {
	case section == PersonalSectionInterface && slices.Contains(nestedInterfaceFields, first),
		section == PersonalSectionUI && slices.Contains(perListUIKeys, first):
		return 2
	}

	return 1
}

// UnitOf returns the unit of the dot separated key. parent is true if the key
// is an object or per-list key without its field or list, which holds several
// units.
func UnitOf(section PersonalSection, key string) (unit string, parent bool) {
	segments := strings.Split(key, ".")
	depth := unitDepth(section, segments[0])
	if len(segments) < depth {
		return key, true
	}

	return strings.Join(segments[:depth], "."), false
}

// Units returns the units of a personal map, sorted by key. Shared keys are
// ignored.
func Units(section PersonalSection, m map[string]interface{}) []PersonalSettingValue {
	var ret []PersonalSettingValue
	add := func(key string, v interface{}) {
		ret = append(ret, PersonalSettingValue{
			PersonalSettingKey: PersonalSettingKey{Section: section, Key: key},
			Value:              v,
		})
	}

	for first, v := range m {
		if IsShared(section, first) {
			continue
		}

		if unitDepth(section, first) == 1 {
			add(first, v)
			continue
		}

		children, _ := v.(map[string]interface{})
		for child, cv := range children {
			add(first+"."+child, cv)
		}
	}

	sort.Slice(ret, func(i, j int) bool {
		return ret[i].Key < ret[j].Key
	})

	return ret
}

// GetUnit returns the value of the unit in the map.
func GetUnit(m map[string]interface{}, unit string) (interface{}, bool) {
	return utils.NestedMap(m).Get(unit)
}

// SetUnit sets the value of the unit in the map, creating its parent.
func SetUnit(m map[string]interface{}, unit string, value interface{}) {
	utils.NestedMap(m).Set(unit, value)
}

// DeleteUnit removes the unit from the map, and its parent if it is left
// empty.
func DeleteUnit(m map[string]interface{}, unit string) {
	utils.NestedMap(m).Delete(unit)

	first := firstSegment(unit)
	if first == unit {
		return
	}

	if parent, ok := m[first].(map[string]interface{}); ok && len(parent) == 0 {
		delete(m, first)
	}
}

// ApplyUnits returns a copy of defaults with the units of personal set.
func ApplyUnits(section PersonalSection, defaults, personal map[string]interface{}) map[string]interface{} {
	ret := copyMap(defaults)
	for _, u := range Units(section, personal) {
		SetUnit(ret, u.Key, deepCopy(u.Value))
	}

	return ret
}

// SetUnits sets the units of values in personal. Each key of values is a unit
// or a parent of units, whose children are set one by one. A nil value
// removes the unit, or all units of a parent, so that they follow the
// defaults. Shared keys are an error.
func SetUnits(section PersonalSection, personal, values map[string]interface{}) error {
	for key := range values {
		if key == "" {
			return errors.New("empty setting key")
		}

		if IsShared(section, key) {
			return fmt.Errorf("%s is shared by every user", key)
		}
	}

	for key, v := range values {
		unit, parent := UnitOf(section, key)
		switch {
		case v == nil && parent:
			delete(personal, key)
		case v == nil:
			DeleteUnit(personal, unit)
		case parent:
			children, ok := v.(map[string]interface{})
			if !ok {
				return fmt.Errorf("%s must be an object", key)
			}

			for child, cv := range children {
				if cv == nil {
					DeleteUnit(personal, key+"."+child)
				} else {
					SetUnit(personal, key+"."+child, deepCopy(cv))
				}
			}
		default:
			SetUnit(personal, unit, deepCopy(v))
		}
	}

	return nil
}

func copyMap(m map[string]interface{}) map[string]interface{} {
	ret := make(map[string]interface{}, len(m))
	for k, v := range m {
		ret[k] = deepCopy(v)
	}

	return ret
}

func deepCopy(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		return copyMap(t)
	case []interface{}:
		ret := make([]interface{}, len(t))
		for i, vv := range t {
			ret[i] = deepCopy(vv)
		}
		return ret
	case []string:
		return slices.Clone(t)
	}

	return v
}

// sameSettings returns true if the settings are equal, an empty map being
// equal to none.
func sameSettings(a, b map[string]interface{}) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}

	return reflect.DeepEqual(a, b)
}

// personalSettingsSize returns the size of the personal settings in bytes of
// JSON.
func personalSettingsSize(iface, ui map[string]interface{}) (int, error) {
	data, err := json.Marshal([]map[string]interface{}{iface, ui})
	if err != nil {
		return 0, err
	}

	return len(data), nil
}

// checkPersonalSettingsSize returns ErrPersonalSettingsTooLarge if the new
// personal settings of the user are larger than MaxPersonalSettingsSize,
// unless they are smaller than the current ones: settings stored before the
// limit can still shrink.
func checkPersonalSettingsSize(u StoredUser, iface, ui map[string]interface{}) error {
	size, err := personalSettingsSize(iface, ui)
	if err != nil {
		return err
	}

	if size <= MaxPersonalSettingsSize {
		return nil
	}

	current, err := personalSettingsSize(u.Interface, u.UI)
	if err != nil {
		return err
	}

	if size >= current {
		return ErrPersonalSettingsTooLarge
	}

	return nil
}

// listedUserNoLock returns the index of the user in the users list, or -1.
// Assumes read lock held.
func (s *UserStore) listedUserNoLock(username string) (int, []StoredUser, error) {
	users, err := s.listedUsersNoLock()
	if err != nil {
		return -1, nil, err
	}

	return s.findUser(users, username), users, nil
}

// IsPersonalUser returns true if the user is in the users list, and so has
// personal settings.
func (s *UserStore) IsPersonalUser(username string) bool {
	s.RLock()
	defer s.RUnlock()

	idx, _, err := s.listedUserNoLock(username)
	return err == nil && idx != -1
}

// PersonalSettings returns copies of the personal interface and UI settings of
// the user. Returns ErrNotPersonal if the user is not in the users list.
func (s *UserStore) PersonalSettings(username string) (iface map[string]interface{}, ui map[string]interface{}, err error) {
	s.RLock()
	defer s.RUnlock()

	idx, users, err := s.listedUserNoLock(username)
	if err != nil {
		return nil, nil, err
	}

	if idx == -1 {
		return nil, nil, ErrNotPersonal
	}

	return copyMap(users[idx].Interface), copyMap(users[idx].UI), nil
}

// UpdatePersonalSettings calls fn with the personal interface and UI settings
// of the user, which fn changes in place, and writes the config file if fn
// succeeds. Returns ErrNotPersonal if the user is not in the users list.
func (s *UserStore) UpdatePersonalSettings(username string, fn func(iface, ui map[string]interface{}) error) error {
	return s.modifyUsers(func(users []StoredUser) ([]StoredUser, error) {
		listed, _, err := s.listedUserNoLock(username)
		if err != nil {
			return nil, err
		}

		if listed == -1 {
			return nil, ErrNotPersonal
		}

		idx := s.findUser(users, username)
		iface := copyMap(users[idx].Interface)
		ui := copyMap(users[idx].UI)
		if err := fn(iface, ui); err != nil {
			return nil, err
		}

		if err := checkPersonalSettingsSize(users[idx], iface, ui); err != nil {
			return nil, err
		}

		if sameSettings(iface, users[idx].Interface) && sameSettings(ui, users[idx].UI) {
			return nil, errUnchanged
		}

		users[idx].Interface = iface
		users[idx].UI = ui
		return users, nil
	})
}

// UpdatePersonalUISettings calls fn with the personal UI settings of the user,
// which fn changes in place, and the shared UI configuration, read under the
// same lock, and writes the config file if fn succeeds.
func (s *UserStore) UpdatePersonalUISettings(username string, fn func(ui, shared map[string]interface{}) error) error {
	return s.UpdatePersonalSettings(username, func(_, ui map[string]interface{}) error {
		// modifyUsers holds the write lock
		return fn(ui, s.forKey(UI).Cut(UI).Raw())
	})
}

// CopyUserData copies the personal settings of the user from to the user to.
// The viewing activity is kept in the database, and is not copied here.
func (s *UserStore) CopyUserData(ctx context.Context, from, to string, settings, activity bool) error {
	if !settings {
		return nil
	}

	return s.CopyPersonalSettings(from, to)
}

// CopyPersonalSettings replaces the personal settings of the user to with
// copies of those of the user from.
func (s *UserStore) CopyPersonalSettings(from, to string) error {
	return s.modifyUsers(func(users []StoredUser) ([]StoredUser, error) {
		src := s.findUser(users, from)
		dst := s.findUser(users, to)
		if src == -1 || dst == -1 {
			return nil, user.ErrUserNotExist
		}

		users[dst].Interface = copyMap(users[src].Interface)
		users[dst].UI = copyMap(users[src].UI)
		return users, nil
	})
}
