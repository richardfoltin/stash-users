package api

import (
	"context"
	"fmt"
	"strconv"

	"github.com/stashapp/stash/internal/api/urlbuilders"
	"github.com/stashapp/stash/internal/manager"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/signedurl"
)

func (r *queryResolver) SceneStreams(ctx context.Context, id *string) ([]*manager.SceneStreamEndpoint, error) {
	sceneID, err := strconv.Atoi(*id)
	if err != nil {
		return nil, err
	}

	// find the scene
	var scene *models.Scene
	if err := r.withReadTxn(ctx, func(ctx context.Context) error {
		var err error
		scene, err = r.repository.Scene.Find(ctx, sceneID)

		if scene != nil {
			err = scene.LoadPrimaryFile(ctx, r.repository.File)
		}

		return err
	}); err != nil {
		return nil, err
	}

	if scene == nil {
		return nil, fmt.Errorf("scene with id %d not found", sceneID)
	}

	config := manager.GetInstance().Config

	baseURL, _ := ctx.Value(BaseURLCtxKey).(string)
	builder := urlbuilders.NewSceneURLBuilder(baseURL, scene)

	streamURL := builder.GetStreamURL("")
	if r.userService.LoginRequired(ctx) {
		currentUser := models.GetCurrentUser(ctx)
		if currentUser == nil {
			return nil, fmt.Errorf("user ID not found")
		}
		streamURL.RawQuery = signedParams(config, currentUser.Username, signedurl.DerivePrefix(streamURL.Path)).Encode()
	} else {
		apiKey := userAPIKey(ctx, config)
		if apiKey != "" {
			v := streamURL.Query()
			v.Set("apikey", apiKey)
			streamURL.RawQuery = v.Encode()
		}
	}

	return manager.GetSceneStreamPaths(scene, streamURL, config.GetMaxStreamingTranscodeSize())
}
