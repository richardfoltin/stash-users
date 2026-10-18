package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/utils"
)

// personalUser returns the name of the current user if the user has personal
// settings, which requires the user to be in the users list. Without a users
// list, every setting is shared.
func personalUser(ctx context.Context) (string, bool) {
	u := models.GetCurrentUser(ctx)
	if u == nil || !config.GetInstance().UserStore.IsPersonalUser(u.Username) {
		return "", false
	}

	return u.Username, true
}

// currentPersonalSettings returns the personal interface and UI settings of
// the current user. Both are nil if the user has no personal settings.
func currentPersonalSettings(ctx context.Context) (iface map[string]interface{}, ui map[string]interface{}) {
	username, ok := personalUser(ctx)
	if !ok {
		return nil, nil
	}

	iface, ui, err := config.GetInstance().UserStore.PersonalSettings(username)
	if err != nil {
		// the user gets the shared settings
		logger.Warnf("reading the personal settings of %s: %v", username, err)
		return nil, nil
	}

	return iface, ui
}

// toJSONMap returns v as a map keyed by its JSON field names.
func toJSONMap(v interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}

	var ret map[string]interface{}
	if err := json.Unmarshal(data, &ret); err != nil {
		return nil, err
	}

	return ret, nil
}

// fromJSONMap sets the fields of v from a map keyed by their JSON field names.
func fromJSONMap(m map[string]interface{}, v interface{}) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, v)
}

// withPersonalInterface returns the shared interface configuration with the
// personal settings applied.
func withPersonalInterface(shared *ConfigInterfaceResult, personal map[string]interface{}) *ConfigInterfaceResult {
	if len(personal) == 0 {
		return shared
	}

	m, err := toJSONMap(shared)
	if err == nil {
		var ret ConfigInterfaceResult
		if err = fromJSONMap(config.ApplyUnits(config.PersonalSectionInterface, m, personal), &ret); err == nil {
			return &ret
		}
	}

	logger.Warnf("applying the personal interface settings: %v", err)
	return shared
}

// makeEffectiveInterfaceResult returns the interface configuration of the
// current user: the shared configuration with their personal settings,
// redacted for non-admins as the configuration query is.
func makeEffectiveInterfaceResult(ctx context.Context) *ConfigInterfaceResult {
	iface, _ := currentPersonalSettings(ctx)
	ret := withPersonalInterface(makeConfigInterfaceResult(), iface)

	if u := models.GetCurrentUser(ctx); u != nil && !u.IsAdmin() {
		redactInterfaceResult(ret)
	}

	return ret
}

// makeEffectiveUIResult returns the UI configuration of the current user: the
// shared configuration with their personal settings.
func makeEffectiveUIResult(ctx context.Context) map[string]interface{} {
	_, ui := currentPersonalSettings(ctx)
	if len(ui) == 0 {
		return makeConfigUIResult()
	}

	return config.ApplyUnits(config.PersonalSectionUI, makeConfigUIResult(), ui)
}

// splitInterfaceInput returns the personal fields set in the input, keyed by
// their GraphQL names, and the shared fields, or nil if none is set.
func splitInterfaceInput(input ConfigInterfaceInput) (map[string]interface{}, *ConfigInterfaceInput, error) {
	m, err := toJSONMap(input)
	if err != nil {
		return nil, nil, err
	}

	// an empty list is omitted from the JSON
	if input.MenuItems != nil {
		m["menuItems"] = input.MenuItems
	}

	personal := make(map[string]interface{})
	sharedValues := make(map[string]interface{})
	for k, v := range m {
		if config.IsShared(config.PersonalSectionInterface, k) {
			sharedValues[k] = v
		} else {
			personal[k] = v
		}
	}

	if len(sharedValues) == 0 {
		return personal, nil, nil
	}

	var shared ConfigInterfaceInput
	if err := fromJSONMap(sharedValues, &shared); err != nil {
		return nil, nil, err
	}

	return personal, &shared, nil
}

// configurePersonalInterface sets the personal fields of the input for the
// current user, and the shared fields, which require the Admin role, for
// everyone.
func (r *mutationResolver) configurePersonalInterface(ctx context.Context, input ConfigInterfaceInput) (*ConfigInterfaceResult, error) {
	username, _ := personalUser(ctx)

	personal, shared, err := splitInterfaceInput(input)
	if err != nil {
		return nil, err
	}

	if shared != nil {
		if err := checkRequirement(ctx, adminRequirement); err != nil {
			return nil, err
		}
	}

	if len(personal) > 0 {
		if err := config.GetInstance().UserStore.UpdatePersonalSettings(username, func(iface, _ map[string]interface{}) error {
			return config.SetUnits(config.PersonalSectionInterface, iface, personal)
		}); err != nil {
			return nil, err
		}
	}

	if shared != nil {
		r.applyInterfaceInput(*shared)
		if err := config.GetInstance().Write(); err != nil {
			return nil, err
		}
	}

	return makeEffectiveInterfaceResult(ctx), nil
}

// configurePersonalUI sets the personal keys of partial for the current user.
// Replacing the entire configuration, and the shared keys, require the Admin
// role.
func (r *mutationResolver) configurePersonalUI(ctx context.Context, input map[string]interface{}, partial map[string]interface{}) (map[string]interface{}, error) {
	username, _ := personalUser(ctx)

	// #5483 - convert JSON numbers to float64 or int64
	partial = convertMapJSONNumbers(partial)

	personal := make(map[string]interface{})
	var shared map[string]interface{}
	for k, v := range partial {
		if config.IsShared(config.PersonalSectionUI, k) {
			if shared == nil {
				shared = make(map[string]interface{})
			}
			shared[k] = v
		} else {
			personal[k] = v
		}
	}

	if input != nil || shared != nil {
		if err := checkRequirement(ctx, adminRequirement); err != nil {
			return nil, err
		}
	}

	// validate the personal keys before changing the shared configuration
	if err := config.SetUnits(config.PersonalSectionUI, make(map[string]interface{}), personal); err != nil {
		return nil, err
	}

	if input != nil || shared != nil {
		if _, err := r.configureSharedUI(input, shared); err != nil {
			return nil, err
		}
	}

	if len(personal) > 0 {
		if err := config.GetInstance().UserStore.UpdatePersonalSettings(username, func(_, ui map[string]interface{}) error {
			return config.SetUnits(config.PersonalSectionUI, ui, personal)
		}); err != nil {
			return nil, err
		}
	}

	return makeEffectiveUIResult(ctx), nil
}

// configurePersonalUISetting sets a key of the personal UI configuration of
// the user. A key below its unit changes a copy of the current value of the
// unit.
func (r *mutationResolver) configurePersonalUISetting(ctx context.Context, username string, key string, value interface{}) (map[string]interface{}, error) {
	if key == "" {
		return nil, errors.New("empty setting key")
	}

	unit, parent := config.UnitOf(config.PersonalSectionUI, key)

	if err := config.GetInstance().UserStore.UpdatePersonalUISettings(username, func(ui, shared map[string]interface{}) error {
		if parent || key == unit {
			return config.SetUnits(config.PersonalSectionUI, ui, map[string]interface{}{key: value})
		}

		current, _ := config.GetUnit(config.ApplyUnits(config.PersonalSectionUI, shared, ui), unit)
		m, ok := current.(map[string]interface{})
		if !ok {
			m = make(map[string]interface{})
		}

		utils.NestedMap(m).Set(strings.TrimPrefix(key, unit+"."), value)
		config.SetUnit(ui, unit, m)
		return nil
	}); err != nil {
		return nil, err
	}

	return makeEffectiveUIResult(ctx), nil
}

func personalSection(s PersonalSettingSection) config.PersonalSection {
	if s == PersonalSettingSectionUI {
		return config.PersonalSectionUI
	}

	return config.PersonalSectionInterface
}

func (r *queryResolver) PersonalSettings(ctx context.Context) ([]*PersonalSetting, error) {
	ret := []*PersonalSetting{}

	iface, ui := currentPersonalSettings(ctx)
	if iface == nil && ui == nil {
		return ret, nil
	}

	sharedInterface, err := toJSONMap(makeConfigInterfaceResult())
	if err != nil {
		return nil, err
	}

	add := func(section PersonalSettingSection, values map[string]interface{}, defaults map[string]interface{}) {
		for _, u := range config.Units(personalSection(section), values) {
			defaultValue, _ := config.GetUnit(defaults, u.Key)
			ret = append(ret, &PersonalSetting{
				Section:      section,
				Key:          u.Key,
				Value:        u.Value,
				DefaultValue: defaultValue,
			})
		}
	}

	add(PersonalSettingSectionInterface, iface, sharedInterface)
	add(PersonalSettingSectionUI, ui, makeConfigUIResult())

	return ret, nil
}

func (r *mutationResolver) ResetPersonalSettings(ctx context.Context, settings []*PersonalSettingInput) (bool, error) {
	username, ok := personalUser(ctx)
	if !ok {
		return true, nil
	}

	if err := config.GetInstance().UserStore.UpdatePersonalSettings(username, func(iface, ui map[string]interface{}) error {
		if settings == nil {
			clear(iface)
			clear(ui)
			return nil
		}

		for _, s := range settings {
			m := iface
			if s.Section == PersonalSettingSectionUI {
				m = ui
			}

			if unit, parent := config.UnitOf(personalSection(s.Section), s.Key); parent {
				delete(m, s.Key)
			} else {
				config.DeleteUnit(m, unit)
			}
		}

		return nil
	}); err != nil {
		return false, err
	}

	return true, nil
}

func (r *mutationResolver) PublishPersonalSettings(ctx context.Context, settings []*PersonalSettingInput) (bool, error) {
	username, ok := personalUser(ctx)
	if !ok {
		return true, nil
	}

	store := config.GetInstance().UserStore
	iface, ui, err := store.PersonalSettings(username)
	if err != nil {
		return false, err
	}

	// check every setting before changing anything
	interfaceValues := make(map[string]interface{})
	uiValues := make(map[string]interface{})
	for _, s := range settings {
		section := personalSection(s.Section)
		values, personal := interfaceValues, iface
		if section == config.PersonalSectionUI {
			values, personal = uiValues, ui
		}

		unit, parent := config.UnitOf(section, s.Key)
		value, found := config.GetUnit(personal, unit)
		if parent || !found || config.IsShared(section, unit) {
			return false, fmt.Errorf("%s is not a personal setting", s.Key)
		}

		req := adminRequirement
		if config.IsListSetting(config.PersonalSettingKey{Section: section, Key: unit}) {
			req = requirement{permission: models.PermissionManageSharedListSettings}
		}

		if err := checkRequirement(ctx, req); err != nil {
			return false, err
		}

		values[unit] = value
	}

	var input ConfigInterfaceInput
	if len(interfaceValues) > 0 {
		nested := make(map[string]interface{})
		for unit, v := range interfaceValues {
			config.SetUnit(nested, unit, v)
		}

		if err := fromJSONMap(nested, &input); err != nil {
			return false, fmt.Errorf("publishing the interface settings: %w", err)
		}
	}

	c := config.GetInstance()
	if len(interfaceValues) > 0 {
		r.applyInterfaceInput(input)
	}

	if len(uiValues) > 0 {
		shared := utils.NestedMap(c.GetUIConfiguration())
		for unit, v := range uiValues {
			shared.Set(unit, v)
		}
		c.SetUIConfiguration(shared)
	}

	if err := c.Write(); err != nil {
		return false, err
	}

	// the published settings are the defaults now
	if err := store.UpdatePersonalSettings(username, func(iface, ui map[string]interface{}) error {
		removePublished(iface, interfaceValues)
		removePublished(ui, uiValues)
		return nil
	}); err != nil {
		return false, err
	}

	return true, nil
}

// removePublished removes the published units from the personal settings,
// unless they were changed since they were read for publishing: the newer
// value stays personal.
func removePublished(personal map[string]interface{}, published map[string]interface{}) {
	for unit, v := range published {
		if current, found := config.GetUnit(personal, unit); found && reflect.DeepEqual(current, v) {
			config.DeleteUnit(personal, unit)
		}
	}
}
