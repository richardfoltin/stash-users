package api

import (
	"context"

	"github.com/stashapp/stash/internal/manager/config"
	"github.com/stashapp/stash/pkg/models"
)

func (r *configResultResolver) Plugins(ctx context.Context, obj *ConfigResult, include []string) (map[string]map[string]interface{}, error) {
	// plugin settings may contain credentials, so only administrators may
	// read them
	if u := models.GetCurrentUser(ctx); u != nil && !u.IsAdmin() {
		return map[string]map[string]interface{}{}, nil
	}

	if len(include) == 0 {
		ret := config.GetInstance().GetAllPluginConfiguration()
		return ret, nil
	}

	ret := make(map[string]map[string]interface{})

	for _, plugin := range include {
		c := config.GetInstance().GetPluginConfiguration(plugin)
		if len(c) > 0 {
			ret[plugin] = c
		}
	}

	return ret, nil
}
