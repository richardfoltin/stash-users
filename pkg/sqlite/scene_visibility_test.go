//go:build integration
// +build integration

package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

// visibility of the scenes that are restricted in the visibility tests
var restrictedScenes = map[int]models.SceneVisibility{
	sceneIdx2WithPerformer: models.SceneVisibilityRestricted,
	sceneIdxWithGroup:      models.SceneVisibilityRestricted,
	sceneIdxWithGallery:    models.SceneVisibilityRestricted,
	sceneIdx2WithStudio:    models.SceneVisibilityPrivate,
	sceneIdxWithTwoTags:    models.SceneVisibilityPrivate,
	sceneIdxWithMarkers:    models.SceneVisibilityPrivate,
}

func restrictScenes(t *testing.T, ctx context.Context) {
	t.Helper()

	for idx, visibility := range restrictedScenes {
		_, err := db.Scene.UpdatePartial(ctx, sceneIDs[idx], models.ScenePartial{Visibility: &visibility})
		require.NoError(t, err)
	}
}

type visibilityContext struct {
	name string
	ctx  func(ctx context.Context) context.Context
	// hides are the visibilities of the scenes that the context does not see
	hides []models.SceneVisibility
}

// userOfRole returns a user with the role with the ID: Admin, or one of the
// default roles.
func userOfRole(name string, roleID string) models.User {
	u := models.User{Username: name, Role: roleID}
	for _, r := range models.DefaultRoles() {
		if r.ID == roleID {
			u.Permissions = r.Permissions
		}
	}
	return u
}

func withRole(roleID string) func(ctx context.Context) context.Context {
	return func(ctx context.Context) context.Context {
		return models.SetCurrentUser(ctx, userOfRole(roleID, roleID))
	}
}

var visibilityContexts = []visibilityContext{
	{"no user", func(ctx context.Context) context.Context { return ctx }, nil},
	{"viewer", withRole(models.RoleIDViewer), []models.SceneVisibility{models.SceneVisibilityRestricted, models.SceneVisibilityPrivate}},
	{"editor", withRole(models.RoleIDEditor), []models.SceneVisibility{models.SceneVisibilityPrivate}},
	{"admin", withRole(models.RoleIDAdmin), nil},
	{"public scenes only", models.SetPublicScenesOnly, []models.SceneVisibility{models.SceneVisibilityRestricted, models.SceneVisibilityPrivate}},
}

// hidden returns true if the restricted scene is hidden in the context.
func (c visibilityContext) hidden(sceneIdx int) bool {
	visibility, restricted := restrictedScenes[sceneIdx]
	return restricted && slices.Contains(c.hides, visibility)
}

// hiddenCount returns the number of the given scenes that are hidden.
func (c visibilityContext) hiddenCount(sceneIdxs ...int) int {
	ret := 0
	for _, idx := range sceneIdxs {
		if c.hidden(idx) {
			ret++
		}
	}
	return ret
}

func restrictedSceneIdxs() []int {
	var ret []int
	for idx := range restrictedScenes {
		ret = append(ret, idx)
	}
	slices.Sort(ret)
	return ret
}

// runVisibilityTest runs the test for each visibility context, with the
// restricted scenes set up in a transaction that is rolled back.
func runVisibilityTest(t *testing.T, f func(t *testing.T, ctx context.Context, c visibilityContext)) {
	withRollbackTxn(func(ctx context.Context) error {
		restrictScenes(t, ctx)

		for _, c := range visibilityContexts {
			t.Run(c.name, func(t *testing.T) {
				f(t, c.ctx(ctx), c)
			})
		}
		return nil
	})
}

func perPageAll() *models.FindFilterType {
	all := -1
	return &models.FindFilterType{PerPage: &all}
}

func TestSceneVisibilityFind(t *testing.T) {
	runVisibilityTest(t, func(t *testing.T, ctx context.Context, c visibilityContext) {
		qb := db.Scene
		restricted := restrictedSceneIdxs()
		nHidden := c.hiddenCount(restricted...)

		// other tests may have added scenes
		nScenes, err := qb.Count(stripUser(ctx))
		require.NoError(t, err)

		for _, idx := range restricted {
			s, err := qb.Find(ctx, sceneIDs[idx])
			require.NoError(t, err)
			assert.Equal(t, c.hidden(idx), s == nil, "Find scene %d", idx)

			byChecksum, err := qb.FindByChecksum(ctx, getSceneStringValue(idx, checksumField))
			require.NoError(t, err)
			assert.Equal(t, c.hidden(idx), len(byChecksum) == 0, "FindByChecksum scene %d", idx)

			byFile, err := qb.GetManyIDsByFileIDs(ctx, []models.FileID{sceneFileIDs[idx]})
			require.NoError(t, err)
			assert.Equal(t, c.hidden(idx), len(byFile[0]) == 0, "GetManyIDsByFileIDs scene %d", idx)
		}

		ids := indexesToIDs(sceneIDs, restricted)
		found, err := qb.FindByIDs(ctx, ids)
		require.NoError(t, err)
		assert.Len(t, found, len(restricted)-nHidden, "FindByIDs")

		_, err = qb.FindMany(ctx, ids)
		assert.Equal(t, nHidden > 0, err != nil, "FindMany error")

		count, err := qb.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, nScenes-nHidden, count, "Count")

		all, err := qb.All(ctx)
		require.NoError(t, err)
		assert.Len(t, all, nScenes-nHidden, "All")

		allWall, err := qb.Wall(stripUser(ctx), nil)
		require.NoError(t, err)
		wall, err := qb.Wall(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, wall, len(allWall)-nHidden, "Wall")

		result, err := qb.Query(ctx, models.SceneQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: perPageAll(), Count: true},
		})
		require.NoError(t, err)
		assert.Equal(t, nScenes-nHidden, result.Count, "Query count")
		for _, idx := range restricted {
			assert.Equal(t, c.hidden(idx), !slices.Contains(result.IDs, sceneIDs[idx]), "Query scene %d", idx)
		}

		// an OR filter must not match hidden scenes
		titleFilter := func(idx int) *models.StringCriterionInput {
			return &models.StringCriterionInput{Value: getSceneTitle(idx), Modifier: models.CriterionModifierEquals}
		}
		orFilter := &models.SceneFilterType{
			Title: titleFilter(sceneIdxWithGroup),
			OperatorFilter: models.OperatorFilter[models.SceneFilterType]{
				Or: &models.SceneFilterType{Title: titleFilter(sceneIdx2WithStudio)},
			},
		}
		orCount, err := qb.QueryCount(ctx, orFilter, nil)
		require.NoError(t, err)
		assert.Equal(t, 2-c.hiddenCount(sceneIdxWithGroup, sceneIdx2WithStudio), orCount, "OR filter count")

		byPerformer, err := qb.FindByPerformerID(ctx, performerIDs[performerIdxWithTwoScenes])
		require.NoError(t, err)
		assert.Len(t, byPerformer, 2-c.hiddenCount(sceneIdx2WithPerformer), "FindByPerformerID")

		byGroup, err := qb.FindByGroupID(ctx, groupIDs[groupIdxWithScene])
		require.NoError(t, err)
		assert.Len(t, byGroup, 1-c.hiddenCount(sceneIdxWithGroup), "FindByGroupID")

		// related scene filters only match visible scenes
		performers, _, err := db.Performer.Query(ctx, &models.PerformerFilterType{
			ScenesFilter: &models.SceneFilterType{Title: titleFilter(sceneIdx2WithPerformer)},
		}, nil)
		require.NoError(t, err)
		assert.Len(t, performers, 1-c.hiddenCount(sceneIdx2WithPerformer), "performers with scenes filter")
	})
}

func TestSceneVisibilityCounts(t *testing.T) {
	withRollbackTxn(func(ctx context.Context) error {
		restrictScenes(t, ctx)

		// history of a hidden and a visible scene of the same performer
		// play counts count distinct dates, so the dates differ
		for i, idx := range []int{sceneIdx1WithPerformer, sceneIdx2WithPerformer} {
			date := time.Now().Add(-time.Duration(i) * time.Hour)
			_, err := db.Scene.AddO(ctx, sceneIDs[idx], []time.Time{date})
			require.NoError(t, err)
			_, err = db.Scene.AddViews(ctx, sceneIDs[idx], []time.Time{date})
			require.NoError(t, err)
		}

		// values with all scenes visible
		qb := db.Scene
		allDuration, err := qb.Duration(ctx)
		require.NoError(t, err)
		allPlayDuration, err := qb.PlayDuration(ctx)
		require.NoError(t, err)
		allOCount, err := qb.GetAllOCount(ctx)
		require.NoError(t, err)
		allViews, err := qb.CountAllViews(ctx)
		require.NoError(t, err)
		allUniqueViews, err := qb.CountUniqueViews(ctx)
		require.NoError(t, err)

		one := models.IntCriterionInput{Value: 1, Modifier: models.CriterionModifierEquals}
		fileSceneCount := func(ctx context.Context) int {
			result, err := db.File.Query(ctx, models.FileQueryOptions{
				QueryOptions: models.QueryOptions{Count: true},
				FileFilter:   &models.FileFilterType{SceneCount: &one},
			})
			require.NoError(t, err)
			return result.Count
		}
		allFileSceneCount := fileSceneCount(ctx)

		for _, c := range visibilityContexts {
			t.Run(c.name, func(t *testing.T) {
				ctx := c.ctx(ctx)
				restricted := restrictedSceneIdxs()

				var hiddenDuration, hiddenPlayDuration float64
				for _, idx := range restricted {
					if c.hidden(idx) {
						hiddenDuration += getSceneDuration(idx)
						hiddenPlayDuration += getScenePlayDuration(idx)
					}
				}
				hiddenHistory := c.hiddenCount(sceneIdx2WithPerformer)

				duration, err := qb.Duration(ctx)
				require.NoError(t, err)
				assert.InDelta(t, allDuration-hiddenDuration, duration, 0.001, "Duration")

				playDuration, err := qb.PlayDuration(ctx)
				require.NoError(t, err)
				assert.InDelta(t, allPlayDuration-hiddenPlayDuration, playDuration, 0.001, "PlayDuration")

				oCount, err := qb.GetAllOCount(ctx)
				require.NoError(t, err)
				assert.Equal(t, allOCount-hiddenHistory, oCount, "GetAllOCount")

				views, err := qb.CountAllViews(ctx)
				require.NoError(t, err)
				assert.Equal(t, allViews-hiddenHistory, views, "CountAllViews")

				uniqueViews, err := qb.CountUniqueViews(ctx)
				require.NoError(t, err)
				assert.Equal(t, allUniqueViews-hiddenHistory, uniqueViews, "CountUniqueViews")

				performerID := performerIDs[performerIdxWithTwoScenes]
				sceneCount, err := qb.CountByPerformerID(ctx, performerID)
				require.NoError(t, err)
				assert.Equal(t, 2-hiddenHistory, sceneCount, "CountByPerformerID")

				performerOCount, err := qb.OCountByPerformerID(ctx, performerID)
				require.NoError(t, err)
				assert.Equal(t, 2-hiddenHistory, performerOCount, "OCountByPerformerID")

				studioCount, err := qb.CountByStudioID(ctx, studioIDs[studioIdxWithTwoScenes])
				require.NoError(t, err)
				assert.Equal(t, 2-c.hiddenCount(sceneIdx2WithStudio), studioCount, "CountByStudioID")

				// count criteria
				two := models.IntCriterionInput{Value: 2, Modifier: models.CriterionModifierEquals}

				performers, _, err := db.Performer.Query(ctx, &models.PerformerFilterType{SceneCount: &two}, perPageAll())
				require.NoError(t, err)
				assert.Equal(t, c.hidden(sceneIdx2WithPerformer), !containsPerformer(performers, performerID), "performer scene_count")

				performers, _, err = db.Performer.Query(ctx, &models.PerformerFilterType{OCounter: &two}, perPageAll())
				require.NoError(t, err)
				assert.Equal(t, c.hidden(sceneIdx2WithPerformer), !containsPerformer(performers, performerID), "performer o_counter")

				performers, _, err = db.Performer.Query(ctx, &models.PerformerFilterType{PlayCount: &two}, perPageAll())
				require.NoError(t, err)
				assert.Equal(t, c.hidden(sceneIdx2WithPerformer), !containsPerformer(performers, performerID), "performer play_count")

				studios, _, err := db.Studio.Query(ctx, &models.StudioFilterType{SceneCount: &two}, perPageAll())
				require.NoError(t, err)
				assert.Equal(t, c.hidden(sceneIdx2WithStudio), !slices.ContainsFunc(studios, func(s *models.Studio) bool {
					return s.ID == studioIDs[studioIdxWithTwoScenes]
				}), "studio scene_count")

				tags, _, err := db.Tag.Query(ctx, &models.TagFilterType{SceneCount: &models.HierarchicalCountInput{Value: 2, Modifier: models.CriterionModifierEquals}}, perPageAll())
				require.NoError(t, err)
				assert.Equal(t, c.hidden(sceneIdxWithTwoTags), !slices.ContainsFunc(tags, func(tag *models.Tag) bool {
					return tag.ID == tagIDs[tagIdx1WithScene]
				}), "tag scene_count")

				groups, _, err := db.Group.Query(ctx, &models.GroupFilterType{SceneCount: &one}, perPageAll())
				require.NoError(t, err)
				assert.Equal(t, c.hidden(sceneIdxWithGroup), !slices.ContainsFunc(groups, func(g *models.Group) bool {
					return g.ID == groupIDs[groupIdxWithScene]
				}), "group scene_count")

				assert.Equal(t, allFileSceneCount-c.hiddenCount(restricted...), fileSceneCount(ctx), "file scene_count")
			})
		}

		return nil
	})
}

func containsPerformer(performers []*models.Performer, id int) bool {
	return slices.ContainsFunc(performers, func(p *models.Performer) bool {
		return p.ID == id
	})
}

// TestSceneVisibilityQueries runs the sorts and criteria that count scenes
// in a restricted context, to ensure that their SQL is valid.
func TestSceneVisibilityQueries(t *testing.T) {
	withRollbackTxn(func(ctx context.Context) error {
		restrictScenes(t, ctx)
		ctx = withRole(models.RoleIDViewer)(ctx)

		sortBy := func(sort string) *models.FindFilterType {
			ret := perPageAll()
			ret.Sort = &sort
			return ret
		}

		for _, sort := range []string{"scenes_count", "scenes_duration", "scenes_size", "play_count", "o_counter", "last_played_at", "last_o_at", "latest_scene"} {
			_, _, err := db.Performer.Query(ctx, nil, sortBy(sort))
			assert.NoError(t, err, "performer sort %s", sort)
		}
		for _, sort := range []string{"scenes_count", "scenes_duration", "scenes_size", "scene_markers_count", "latest_scene", "o_counter", "o_counter_all", "performer_count", "performer_count_all"} {
			_, _, err := db.Studio.Query(ctx, nil, sortBy(sort))
			assert.NoError(t, err, "studio sort %s", sort)
		}
		for _, sort := range []string{"scenes_count", "scenes_duration", "scenes_size", "scene_markers_count"} {
			_, _, err := db.Tag.Query(ctx, nil, sortBy(sort))
			assert.NoError(t, err, "tag sort %s", sort)
		}
		for _, sort := range []string{"scenes_count", "o_counter"} {
			_, _, err := db.Group.Query(ctx, nil, sortBy(sort))
			assert.NoError(t, err, "group sort %s", sort)
		}

		one := models.IntCriterionInput{Value: 1, Modifier: models.CriterionModifierGreaterThan}
		depth := -1
		_, _, err := db.Performer.Query(ctx, &models.PerformerFilterType{
			MarkerCount: &one,
			Studios:     &models.HierarchicalMultiCriterionInput{Value: []string{"1"}, Modifier: models.CriterionModifierIncludes, Excludes: []string{"2"}},
			Groups:      &models.HierarchicalMultiCriterionInput{Value: []string{"1"}, Modifier: models.CriterionModifierIncludes, Depth: &depth},
			Performers:  &models.MultiCriterionInput{Value: []string{"1"}, Modifier: models.CriterionModifierIncludes},
		}, nil)
		assert.NoError(t, err, "performer criteria")

		_, _, err = db.Group.Query(ctx, &models.GroupFilterType{
			OCounter:   &one,
			Performers: &models.MultiCriterionInput{Value: []string{"1"}, Modifier: models.CriterionModifierIncludes},
		}, nil)
		assert.NoError(t, err, "group criteria")

		_, _, err = db.Tag.Query(ctx, &models.TagFilterType{
			MarkerCount: &models.HierarchicalCountInput{Value: 0, Modifier: models.CriterionModifierGreaterThan, Depth: &depth},
			SceneCount:  &models.HierarchicalCountInput{Value: 0, Modifier: models.CriterionModifierGreaterThan, Depth: &depth},
		}, nil)
		assert.NoError(t, err, "tag criteria")

		duplicated := true
		_, err = db.Scene.QueryCount(ctx, &models.SceneFilterType{
			Duplicated: &models.DuplicationCriterionInput{Phash: &duplicated, StashID: &duplicated, Title: &duplicated, URL: &duplicated},
		}, nil)
		assert.NoError(t, err, "scene duplicated criteria")

		_, err = db.Scene.FindDuplicates(ctx, 0, -1, nil)
		assert.NoError(t, err, "FindDuplicates")

		_, err = db.Group.FindByPerformerID(ctx, performerIDs[performerIdxWithTwoScenes])
		assert.NoError(t, err, "group FindByPerformerID")
		_, err = db.Group.CountByPerformerID(ctx, performerIDs[performerIdxWithTwoScenes])
		assert.NoError(t, err, "group CountByPerformerID")
		_, err = db.Scene.OCountByStudioID(ctx, studioIDs[studioIdxWithTwoScenes], -1)
		assert.NoError(t, err, "OCountByStudioID recursive")
		_, err = db.Scene.OCountByGroupID(ctx, groupIDs[groupIdxWithScene])
		assert.NoError(t, err, "OCountByGroupID")

		return nil
	})
}

func TestSceneVisibilityMarkers(t *testing.T) {
	// indexes of the markers of the scene with markers
	const sceneMarkers = 4
	markerTagID := func() int { return tagIDs[tagIdxWithMarkers] }

	runVisibilityTest(t, func(t *testing.T, ctx context.Context, c visibilityContext) {
		qb := db.SceneMarker
		hidden := c.hidden(sceneIdxWithMarkers)
		nHidden := 0
		if hidden {
			nHidden = sceneMarkers
		}

		bySceneID, err := qb.FindBySceneID(ctx, sceneIDs[sceneIdxWithMarkers])
		require.NoError(t, err)
		assert.Len(t, bySceneID, sceneMarkers-nHidden, "FindBySceneID")

		m, err := qb.Find(ctx, markerIDs[0])
		require.NoError(t, err)
		assert.Equal(t, hidden, m == nil, "Find")

		count, err := qb.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, len(markerIDs)-nHidden, count, "Count")

		_, queryCount, err := qb.Query(ctx, nil, perPageAll())
		require.NoError(t, err)
		assert.Equal(t, len(markerIDs)-nHidden, queryCount, "Query count")

		wall, err := qb.Wall(ctx, nil)
		require.NoError(t, err)
		assert.Len(t, wall, len(markerIDs)-nHidden, "Wall")

		all, err := qb.All(ctx)
		require.NoError(t, err)
		assert.Len(t, all, len(markerIDs)-nHidden, "All")

		// two markers of the scene with markers have the tag
		tagCount, err := qb.CountByTagID(ctx, markerTagID())
		require.NoError(t, err)
		expectedTagCount := 2
		if hidden {
			expectedTagCount = 0
		}
		assert.Equal(t, expectedTagCount, tagCount, "CountByTagID")

		markerStrings, err := qb.GetMarkerStrings(ctx, nil, nil)
		require.NoError(t, err)
		total := 0
		for _, s := range markerStrings {
			total += s.Count
		}
		assert.Equal(t, len(markerIDs)-nHidden, total, "GetMarkerStrings")

		tags, _, err := db.Tag.Query(ctx, &models.TagFilterType{
			MarkerCount: &models.HierarchicalCountInput{Value: 2, Modifier: models.CriterionModifierEquals},
		}, perPageAll())
		require.NoError(t, err)
		assert.Equal(t, hidden, !slices.ContainsFunc(tags, func(tag *models.Tag) bool {
			return tag.ID == markerTagID()
		}), "tag marker_count")

		// markers of hidden scenes cannot be created or changed
		newMarker := models.SceneMarker{
			Title:        "visibility test",
			SceneID:      sceneIDs[sceneIdxWithMarkers],
			PrimaryTagID: tagIDs[tagIdxWithPrimaryMarkers],
		}
		err = qb.Create(ctx, &newMarker)
		assertNotFound(t, hidden, err, "Create")
		if err == nil {
			require.NoError(t, qb.Destroy(ctx, newMarker.ID))
		}

		title := "changed"
		_, err = qb.UpdatePartial(ctx, markerIDs[0], models.SceneMarkerPartial{Title: models.NewOptionalString(title)})
		assertNotFound(t, hidden, err, "UpdatePartial")
	})
}

// assertNotFound asserts that err is a not found error if notFound is true,
// and nil otherwise.
func assertNotFound(t *testing.T, notFound bool, err error, msg string) {
	t.Helper()

	if notFound {
		var nfErr *sqlite.NotFoundError
		assert.True(t, errors.As(err, &nfErr), "%s: expected not found error, got %v", msg, err)
	} else {
		assert.NoError(t, err, msg)
	}
}

func TestSceneVisibilityWrites(t *testing.T) {
	writes := []struct {
		name string
		fn   func(ctx context.Context, id int) error
	}{
		{"UpdatePartial", func(ctx context.Context, id int) error {
			_, err := db.Scene.UpdatePartial(ctx, id, models.ScenePartial{Title: models.NewOptionalString("changed")})
			return err
		}},
		{"SaveActivity", func(ctx context.Context, id int) error {
			resume := 1.0
			_, err := db.Scene.SaveActivity(ctx, id, &resume, nil)
			return err
		}},
		{"ResetActivity", func(ctx context.Context, id int) error {
			_, err := db.Scene.ResetActivity(ctx, id, true, true)
			return err
		}},
		{"AddViews", func(ctx context.Context, id int) error {
			_, err := db.Scene.AddViews(ctx, id, nil)
			return err
		}},
		{"DeleteViews", func(ctx context.Context, id int) error {
			_, err := db.Scene.DeleteViews(ctx, id, nil)
			return err
		}},
		{"DeleteAllViews", func(ctx context.Context, id int) error {
			_, err := db.Scene.DeleteAllViews(ctx, id)
			return err
		}},
		{"AddO", func(ctx context.Context, id int) error {
			_, err := db.Scene.AddO(ctx, id, nil)
			return err
		}},
		{"DeleteO", func(ctx context.Context, id int) error {
			_, err := db.Scene.DeleteO(ctx, id, nil)
			return err
		}},
		{"ResetO", func(ctx context.Context, id int) error {
			_, err := db.Scene.ResetO(ctx, id)
			return err
		}},
	}

	runVisibilityTest(t, func(t *testing.T, ctx context.Context, c visibilityContext) {
		for _, idx := range []int{sceneIdx2WithPerformer, sceneIdx2WithStudio} {
			for _, w := range writes {
				err := w.fn(ctx, sceneIDs[idx])
				assertNotFound(t, c.hidden(idx), err, w.name)
			}
		}
	})
}

func TestSceneVisibilityFiles(t *testing.T) {
	runVisibilityTest(t, func(t *testing.T, ctx context.Context, c visibilityContext) {
		for _, idx := range []int{sceneIdx2WithStudio, sceneIdxWithTag} {
			fileID := sceneFileIDs[idx]
			_, err := db.File.Find(ctx, fileID)
			assert.Equal(t, c.hidden(idx), err != nil, "Find file of scene %d", idx)

			result, err := db.File.Query(ctx, models.FileQueryOptions{
				QueryOptions: models.QueryOptions{Count: true},
				FileFilter: &models.FileFilterType{
					Basename: &models.StringCriterionInput{Value: getSceneBasename(idx), Modifier: models.CriterionModifierEquals},
				},
			})
			require.NoError(t, err)
			expected := 1
			if c.hidden(idx) {
				expected = 0
			}
			assert.Equal(t, expected, result.Count, "Query file of scene %d", idx)
		}

		// the files of visible scenes can be loaded
		s, err := db.Scene.Find(ctx, sceneIDs[sceneIdxWithTag])
		require.NoError(t, err)
		require.NoError(t, s.LoadFiles(ctx, db.Scene))
		assert.Len(t, s.Files.List(), 1)
	})
}

func TestSceneVisibilityGalleries(t *testing.T) {
	runVisibilityTest(t, func(t *testing.T, ctx context.Context, c visibilityContext) {
		galleryID := galleryIDs[galleryIdxWithScene]
		hiddenSceneID := sceneIDs[sceneIdxWithGallery]
		hidden := c.hidden(sceneIdxWithGallery)

		sceneIDsOfGallery, err := db.Gallery.GetSceneIDs(ctx, galleryID)
		require.NoError(t, err)
		assert.Equal(t, hidden, !slices.Contains(sceneIDsOfGallery, hiddenSceneID), "GetSceneIDs")

		// hidden scenes cannot be linked
		_, err = db.Gallery.UpdatePartial(ctx, galleryID, models.GalleryPartial{
			SceneIDs: &models.UpdateIDs{IDs: []int{sceneIDs[sceneIdxWithTwoTags]}, Mode: models.RelationshipUpdateModeAdd},
		})
		assertNotFound(t, c.hidden(sceneIdxWithTwoTags), err, "link hidden scene")

		// setting the scenes keeps the links to hidden scenes
		visibleSceneID := sceneIDs[sceneIdxWithTag]
		_, err = db.Gallery.UpdatePartial(ctx, galleryID, models.GalleryPartial{
			SceneIDs: &models.UpdateIDs{IDs: []int{visibleSceneID}, Mode: models.RelationshipUpdateModeSet},
		})
		require.NoError(t, err)

		linked, err := db.Gallery.GetSceneIDs(context.WithoutCancel(stripUser(ctx)), galleryID)
		require.NoError(t, err)
		assert.Contains(t, linked, visibleSceneID)
		assert.Equal(t, hidden, slices.Contains(linked, hiddenSceneID), "link to hidden scene kept")

		// restore the fixture
		_, err = db.Gallery.UpdatePartial(stripUser(ctx), galleryID, models.GalleryPartial{
			SceneIDs: &models.UpdateIDs{IDs: []int{hiddenSceneID}, Mode: models.RelationshipUpdateModeSet},
		})
		require.NoError(t, err)
	})
}

// stripUser returns a context that sees all scenes.
func stripUser(ctx context.Context) context.Context {
	return models.SetAllScenesVisible(ctx)
}

func TestSceneVisibilityFilter(t *testing.T) {
	withRollbackTxn(func(ctx context.Context) error {
		restrictScenes(t, ctx)

		count := func(modifier models.CriterionModifier, visibilities ...models.SceneVisibility) int {
			n, err := db.Scene.QueryCount(ctx, &models.SceneFilterType{
				Visibility: &models.SceneVisibilityCriterionInput{Value: visibilities, Modifier: modifier},
			}, nil)
			require.NoError(t, err)
			return n
		}

		assert.Equal(t, 3, count(models.CriterionModifierIncludes, models.SceneVisibilityPrivate))
		assert.Equal(t, 3, count(models.CriterionModifierEquals, models.SceneVisibilityRestricted))
		assert.Equal(t, 6, count(models.CriterionModifierIncludes, models.SceneVisibilityRestricted, models.SceneVisibilityPrivate))
		nScenes, err := db.Scene.Count(ctx)
		require.NoError(t, err)
		assert.Equal(t, nScenes-6, count(models.CriterionModifierIncludes, models.SceneVisibilityEveryone))
		assert.Equal(t, 6, count(models.CriterionModifierExcludes, models.SceneVisibilityEveryone))

		visibility, err := db.Scene.GetVisibility(ctx, sceneIDs[sceneIdxWithMarkers])
		require.NoError(t, err)
		assert.Equal(t, models.SceneVisibilityPrivate, visibility)

		visibility, err = db.Scene.GetVisibility(ctx, sceneIDs[sceneIdxWithTag])
		require.NoError(t, err)
		assert.Equal(t, models.SceneVisibilityEveryone, visibility)

		visibilities, err := db.Scene.GetManyVisibility(ctx, []int{sceneIDs[sceneIdxWithTag], sceneIDs[sceneIdxWithMarkers], sceneIDs[sceneIdx2WithPerformer]})
		require.NoError(t, err)
		assert.Equal(t, []models.SceneVisibility{models.SceneVisibilityEveryone, models.SceneVisibilityPrivate, models.SceneVisibilityRestricted}, visibilities)

		// setting EVERYONE removes the restriction
		everyone := models.SceneVisibilityEveryone
		_, err = db.Scene.UpdatePartial(ctx, sceneIDs[sceneIdxWithMarkers], models.ScenePartial{Visibility: &everyone})
		require.NoError(t, err)
		assert.Equal(t, 2, count(models.CriterionModifierIncludes, models.SceneVisibilityPrivate))

		return nil
	})
}

func TestSceneVisibilityDefault(t *testing.T) {
	db.SetDefaultSceneVisibility(func() models.SceneVisibility { return models.SceneVisibilityPrivate })
	t.Cleanup(func() { db.SetDefaultSceneVisibility(nil) })

	withRollbackTxn(func(ctx context.Context) error {
		s := models.Scene{Title: "visibility default"}
		require.NoError(t, db.Scene.Create(ctx, &s, nil))

		visibility, err := db.Scene.GetVisibility(ctx, s.ID)
		require.NoError(t, err)
		assert.Equal(t, models.SceneVisibilityPrivate, visibility)

		found, err := db.Scene.Find(withRole(models.RoleIDEditor)(ctx), s.ID)
		require.NoError(t, err)
		assert.Nil(t, found, "editor sees the new scene")

		// destroying the scene removes its visibility
		require.NoError(t, db.Scene.Destroy(ctx, s.ID))
		_, rows, err := db.QuerySQL(ctx, "SELECT scene_id FROM scene_visibility WHERE scene_id = ?", []interface{}{s.ID})
		require.NoError(t, err)
		assert.Empty(t, rows)

		return nil
	})
}

// TestSceneVisibilitySurvivesScenesRebuild rebuilds the scenes table as the
// migrations do - with foreign keys disabled, creating a new table and
// renaming it to scenes - and checks that the visibility survives.
func TestSceneVisibilitySurvivesScenesRebuild(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "rebuild.sqlite")

	testDB := sqlite.NewDatabase()
	require.NoError(t, testDB.Open(dbPath))

	var sceneID int
	require.NoError(t, txn.WithTxn(context.Background(), testDB, func(ctx context.Context) error {
		s := models.Scene{Title: "rebuilt"}
		if err := testDB.Scene.Create(ctx, &s, nil); err != nil {
			return err
		}
		sceneID = s.ID

		private := models.SceneVisibilityPrivate
		_, err := testDB.Scene.UpdatePartial(ctx, s.ID, models.ScenePartial{Visibility: &private})
		return err
	}))
	require.NoError(t, testDB.Close())

	// foreign keys are disabled by default
	conn, err := sql.Open("sqlite3", "file:"+dbPath)
	require.NoError(t, err)

	var createSQL string
	require.NoError(t, conn.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'scenes'").Scan(&createSQL))
	createSQL = regexp.MustCompile("^CREATE TABLE [`\"]?scenes[`\"]?").ReplaceAllString(createSQL, "CREATE TABLE scenes_new")

	for _, stmt := range []string{
		createSQL,
		"INSERT INTO scenes_new SELECT * FROM scenes",
		"DROP TABLE scenes",
		"ALTER TABLE scenes_new RENAME TO scenes",
	} {
		_, err := conn.Exec(stmt)
		require.NoError(t, err, stmt)
	}
	require.NoError(t, conn.Close())

	testDB = sqlite.NewDatabase()
	require.NoError(t, testDB.Open(dbPath))
	defer testDB.Close()

	require.NoError(t, txn.WithTxn(context.Background(), testDB, func(ctx context.Context) error {
		visibility, err := testDB.Scene.GetVisibility(ctx, sceneID)
		require.NoError(t, err)
		assert.Equal(t, models.SceneVisibilityPrivate, visibility, "visibility after rebuild")

		found, err := testDB.Scene.Find(withRole(models.RoleIDViewer)(ctx), sceneID)
		require.NoError(t, err)
		assert.Nil(t, found, "viewer sees the scene after rebuild")

		// the reference to scenes still cascades
		require.NoError(t, testDB.Scene.Destroy(ctx, sceneID))
		_, rows, err := testDB.QuerySQL(ctx, "SELECT scene_id FROM scene_visibility", nil)
		require.NoError(t, err)
		assert.Empty(t, rows, "visibility rows after destroy")

		return nil
	}))
}

// TestSceneVisibility_CustomRole checks that the two permissions decide
// independently: a role can see private scenes without seeing restricted ones.
func TestSceneVisibility_CustomRole(t *testing.T) {
	withRollbackTxn(func(ctx context.Context) error {
		restrictScenes(t, ctx)
		ctx = models.SetCurrentUser(ctx, models.User{Username: "custom", Role: "custom", Permissions: models.Permissions{models.PermissionViewPrivateScenes}})

		for idx, visible := range map[int]bool{
			sceneIdxWithTag:        true,  // everyone
			sceneIdxWithMarkers:    true,  // private
			sceneIdx2WithPerformer: false, // restricted
		} {
			s, err := db.Scene.Find(ctx, sceneIDs[idx])
			require.NoError(t, err)
			assert.Equal(t, visible, s != nil, "scene %d visible", idx)
		}

		return nil
	})
}

// TestSceneVisibilityCriterion_LegacyValue filters by MODIFY, a value of
// filters saved before the visibilities were named.
func TestSceneVisibilityCriterion_LegacyValue(t *testing.T) {
	withRollbackTxn(func(ctx context.Context) error {
		restrictScenes(t, ctx)

		visibility, ok := models.ParseSceneVisibility("MODIFY")
		require.True(t, ok)

		result, err := db.Scene.Query(ctx, models.SceneQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: perPageAll()},
			SceneFilter: &models.SceneFilterType{
				Visibility: &models.SceneVisibilityCriterionInput{Value: []models.SceneVisibility{visibility}, Modifier: models.CriterionModifierIncludes},
			},
		})
		require.NoError(t, err)
		assert.ElementsMatch(t, []int{sceneIDs[sceneIdx2WithPerformer], sceneIDs[sceneIdxWithGroup], sceneIDs[sceneIdxWithGallery]}, result.IDs)

		return nil
	})
}
