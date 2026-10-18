package api

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/utils"
)

func (r *queryResolver) FindSavedFilter(ctx context.Context, id string) (ret *models.SavedFilter, err error) {
	idInt, err := strconv.Atoi(id)
	if err != nil {
		return nil, err
	}

	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		if models.IsPersonalSavedFilterID(idInt) {
			// only its owner sees a personal filter
			user := personalFilterUser(ctx)
			if user == "" {
				return nil
			}

			ret, err = r.repository.PersonalSavedFilter.Find(ctx, user, idInt)
			return err
		}

		ret, err = r.repository.SavedFilter.Find(ctx, idInt)
		return err
	}); err != nil {
		return nil, err
	}
	return ret, err
}

func (r *queryResolver) FindSavedFilters(ctx context.Context, mode *models.FilterMode) (ret []*models.SavedFilter, err error) {
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		var shared, personal []*models.SavedFilter
		user := personalFilterUser(ctx)

		if mode != nil {
			shared, err = r.repository.SavedFilter.FindByMode(ctx, *mode)
			if err == nil && user != "" {
				personal, err = r.repository.PersonalSavedFilter.FindByMode(ctx, user, *mode)
			}
		} else {
			shared, err = r.repository.SavedFilter.All(ctx)
			if err == nil && user != "" {
				personal, err = r.repository.PersonalSavedFilter.All(ctx, user)
			}
		}
		if err != nil {
			return err
		}

		ret = mergeSavedFilters(personal, shared)
		return nil
	}); err != nil {
		return nil, err
	}
	return ret, err
}

// mergeSavedFilters returns the personal and the shared filters sorted by name
// in byte order, as the database sorts them, with a personal filter first on
// equal names.
func mergeSavedFilters(personal []*models.SavedFilter, shared []*models.SavedFilter) []*models.SavedFilter {
	ret := append(append([]*models.SavedFilter{}, personal...), shared...)
	sort.SliceStable(ret, func(i, j int) bool {
		return ret[i].Name < ret[j].Name
	})

	return ret
}

func (r *queryResolver) FindDefaultFilter(ctx context.Context, mode models.FilterMode) (ret *models.SavedFilter, err error) {
	// deprecated - read from the config of the current user in the meantime
	uiConfig := makeEffectiveUIResult(ctx)
	if uiConfig == nil {
		return nil, nil
	}

	m := utils.NestedMap(uiConfig)
	filterRaw, _ := m.Get("defaultFilters." + strings.ToLower(mode.String()))

	if filterRaw == nil {
		return nil, nil
	}

	ret = &models.SavedFilter{}
	d, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          "json",
		WeaklyTypedInput: true,
		Result:           ret,
	})

	if err != nil {
		return nil, err
	}

	if err := d.Decode(filterRaw); err != nil {
		return nil, err
	}

	return ret, nil
}
