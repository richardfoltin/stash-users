package api

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mitchellh/mapstructure"
	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/utils"
)

var errPersonalFilterUser = errors.New("personal saved filters need a signed-in user")

// personalFilterUser returns the name of the current user, whose personal
// saved filters the request reads and writes, or an empty string if there is
// none.
func personalFilterUser(ctx context.Context) string {
	if u := models.GetCurrentUser(ctx); u != nil {
		return u.Username
	}

	return ""
}

// nameTaken returns true if a filter other than f has the mode and name of f.
func nameTaken(filters []*models.SavedFilter, f models.SavedFilter) bool {
	for _, o := range filters {
		if o.ID != f.ID && o.Mode == f.Mode && o.Name == f.Name {
			return true
		}
	}

	return false
}

func (r *mutationResolver) SaveFilter(ctx context.Context, input SaveFilterInput) (*models.SavedFilter, error) {
	if strings.TrimSpace(input.Name) == "" {
		return nil, errors.New("name must be non-empty")
	}

	f := models.SavedFilter{
		Mode:         input.Mode,
		Name:         strings.TrimSpace(input.Name),
		FindFilter:   input.FindFilter,
		ObjectFilter: input.ObjectFilter,
		UIOptions:    input.UIOptions,
	}

	// a new filter is personal if asked, an existing one stays what it is
	creating := input.ID == nil
	personal := input.Personal != nil && *input.Personal
	if !creating {
		id, err := strconv.Atoi(*input.ID)
		if err != nil {
			return nil, fmt.Errorf("converting id: %w", err)
		}
		f.ID = id
		personal = models.IsPersonalSavedFilterID(id)
	}

	if personal {
		return r.savePersonalFilter(ctx, f, creating)
	}

	// the shared filters need the permission
	if err := checkRequirement(ctx, requirement{permission: models.PermissionSaveFilters}); err != nil {
		return nil, err
	}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		qb := r.repository.SavedFilter

		filters, err := qb.FindByMode(ctx, f.Mode)
		if err != nil {
			return err
		}
		if nameTaken(filters, f) {
			return fmt.Errorf("a saved filter named %q already exists", f.Name)
		}

		if creating {
			return qb.Create(ctx, &f)
		}

		return qb.Update(ctx, &f)
	}); err != nil {
		return nil, err
	}

	return &f, nil
}

// savePersonalFilter creates or updates a personal filter of the current user.
func (r *mutationResolver) savePersonalFilter(ctx context.Context, f models.SavedFilter, creating bool) (*models.SavedFilter, error) {
	user := personalFilterUser(ctx)
	if user == "" {
		return nil, errPersonalFilterUser
	}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		qb := r.repository.PersonalSavedFilter

		if !creating {
			// the filter of another user is not found
			existing, err := qb.Find(ctx, user, f.ID)
			if err != nil {
				return err
			}
			if existing == nil {
				return fmt.Errorf("saved filter %d not found", f.ID)
			}
		}

		filters, err := qb.FindByMode(ctx, user, f.Mode)
		if err != nil {
			return err
		}
		if nameTaken(filters, f) {
			return fmt.Errorf("a saved filter named %q already exists", f.Name)
		}

		if creating {
			return qb.Create(ctx, user, &f)
		}

		return qb.Update(ctx, user, &f)
	}); err != nil {
		return nil, err
	}

	f.Personal = true
	return &f, nil
}

func (r *mutationResolver) DestroySavedFilter(ctx context.Context, input DestroyFilterInput) (bool, error) {
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return false, fmt.Errorf("converting id: %w", err)
	}

	if models.IsPersonalSavedFilterID(id) {
		user := personalFilterUser(ctx)
		if user == "" {
			return false, errPersonalFilterUser
		}

		if err := r.withTxn(ctx, func(ctx context.Context) error {
			qb := r.repository.PersonalSavedFilter

			// the filter of another user is not found
			existing, err := qb.Find(ctx, user, id)
			if err != nil {
				return err
			}
			if existing == nil {
				return fmt.Errorf("saved filter %d not found", id)
			}

			return qb.Destroy(ctx, user, id)
		}); err != nil {
			return false, err
		}

		return true, nil
	}

	// the shared filters need the permission
	if err := checkRequirement(ctx, requirement{permission: models.PermissionDeleteSavedFilters}); err != nil {
		return false, err
	}

	if err := r.withTxn(ctx, func(ctx context.Context) error {
		return r.repository.SavedFilter.Destroy(ctx, id)
	}); err != nil {
		return false, err
	}

	return true, nil
}

func (r *mutationResolver) ShareSavedFilter(ctx context.Context, input ShareSavedFilterInput) (*models.SavedFilter, error) {
	id, err := strconv.Atoi(input.ID)
	if err != nil {
		return nil, fmt.Errorf("converting id: %w", err)
	}

	if !models.IsPersonalSavedFilterID(id) {
		return nil, fmt.Errorf("saved filter %d is not a personal filter", id)
	}

	user := personalFilterUser(ctx)
	if user == "" {
		return nil, errPersonalFilterUser
	}

	var ret *models.SavedFilter
	if err := r.withTxn(ctx, func(ctx context.Context) error {
		personal, err := r.repository.PersonalSavedFilter.Find(ctx, user, id)
		if err != nil {
			return err
		}
		if personal == nil {
			return fmt.Errorf("saved filter %d not found", id)
		}

		shared, err := r.repository.SavedFilter.FindByMode(ctx, personal.Mode)
		if err != nil {
			return err
		}

		f := *personal
		f.ID = 0
		f.Personal = false
		if nameTaken(shared, f) {
			return fmt.Errorf("a shared filter named %q already exists", f.Name)
		}

		if err := r.repository.SavedFilter.Create(ctx, &f); err != nil {
			return err
		}
		if err := r.repository.PersonalSavedFilter.Destroy(ctx, user, id); err != nil {
			return err
		}

		ret = &f
		return nil
	}); err != nil {
		return nil, err
	}

	// the filter keeps its row on the front page of the user
	if err := replaceFrontPageFilter(ctx, id, ret.ID); err != nil {
		logger.Warnf("replacing the shared filter on the front page: %v", err)
	}

	return ret, nil
}

func (r *mutationResolver) SetDefaultFilter(ctx context.Context, input SetDefaultFilterInput) (bool, error) {
	// deprecated - write to the config in the meantime
	config := config.GetInstance()

	uiConfig := config.GetUIConfiguration()
	if uiConfig == nil {
		uiConfig = make(map[string]interface{})
	}

	m := utils.NestedMap(uiConfig)

	if input.FindFilter == nil && input.ObjectFilter == nil && input.UIOptions == nil {
		// clearing
		m.Delete("defaultFilters." + strings.ToLower(input.Mode.String()))
		config.SetUIConfiguration(m)

		if err := config.Write(); err != nil {
			return false, err
		}

		return true, nil
	}

	subMap := make(map[string]interface{})
	d, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		TagName:          "json",
		WeaklyTypedInput: true,
		Result:           &subMap,
	})

	if err != nil {
		return false, err
	}

	if err := d.Decode(input); err != nil {
		return false, err
	}

	m.Set("defaultFilters."+strings.ToLower(input.Mode.String()), subMap)

	config.SetUIConfiguration(m)

	if err := config.Write(); err != nil {
		return false, err
	}

	return true, nil
}
