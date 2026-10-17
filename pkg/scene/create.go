package scene

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/plugin/hook"
)

func (s *Service) Create(ctx context.Context, input models.CreateSceneInput) (*models.Scene, error) {
	// title must be set if no files are provided
	if input.Scene.Title == "" && len(input.FileIDs) == 0 {
		return nil, errors.New("title must be set if scene has no files")
	}

	now := time.Now()
	newScene := *input.Scene
	newScene.CreatedAt = now
	newScene.UpdatedAt = now

	// the new scene is set up even if its default visibility hides it from
	// its creator; the files to assign are still subject to the visibility
	createCtx := models.SetAllScenesVisible(ctx)

	// don't pass the file ids since they may be already assigned
	// assign them afterwards
	if err := s.Repository.Create(createCtx, &newScene, nil); err != nil {
		return nil, fmt.Errorf("creating new scene: %w", err)
	}

	if len(input.CustomFields) > 0 {
		if err := s.Repository.SetCustomFields(createCtx, newScene.ID, models.CustomFieldsInput{
			Full: input.CustomFields,
		}); err != nil {
			return nil, fmt.Errorf("setting custom fields on new scene: %w", err)
		}
	}

	for _, f := range input.FileIDs {
		if err := s.AssignFile(ctx, newScene.ID, f); err != nil {
			return nil, fmt.Errorf("assigning file %d to new scene: %w", f, err)
		}
	}

	if len(input.FileIDs) > 0 {
		// assign the primary to the first
		if _, err := s.Repository.UpdatePartial(createCtx, newScene.ID, models.ScenePartial{
			PrimaryFileID: &input.FileIDs[0],
		}); err != nil {
			return nil, fmt.Errorf("setting primary file on new scene: %w", err)
		}
	}

	// re-find the scene so that it correctly returns file-related fields
	ret, err := s.Repository.Find(createCtx, newScene.ID)
	if err != nil {
		return nil, err
	}

	if len(input.CoverImage) > 0 {
		if err := s.Repository.UpdateCover(createCtx, ret.ID, input.CoverImage); err != nil {
			return nil, fmt.Errorf("setting cover on new scene: %w", err)
		}
	}

	s.PluginCache.RegisterPostHooks(ctx, ret.ID, hook.SceneCreatePost, nil, nil)

	// the scene is not returned if it is hidden from its creator
	if len(models.HiddenSceneVisibilities(ctx)) > 0 {
		return s.Repository.Find(ctx, ret.ID)
	}

	// re-find the scene so that it correctly returns file-related fields
	return ret, nil
}
