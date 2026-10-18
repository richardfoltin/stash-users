//go:build integration
// +build integration

package sqlite_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/scene"
	"github.com/stashapp/stash/pkg/sqlite"
	"github.com/stashapp/stash/pkg/txn"
)

// activityDB is a database for the user activity tests. It is separate from
// the shared test database, so that the first admin and the adoption of the
// legacy activity can be set freely.
type activityDB struct {
	*sqlite.Database
	path string
	// admin is the name of the first admin, empty if there is none
	admin string

	scenes    []int
	images    []int
	performer int
	studio    int
	group     int
}

func newActivityDB(t *testing.T) *activityDB {
	t.Helper()

	ret := &activityDB{
		Database: sqlite.NewDatabase(),
		path:     filepath.Join(t.TempDir(), "activity.sqlite"),
	}
	ret.SetBlobStoreOptions(sqlite.BlobStoreOptions{UseDatabase: true})
	ret.SetFirstAdmin(func(ctx context.Context) string {
		return ret.admin
	})

	require.NoError(t, ret.Open(ret.path))
	t.Cleanup(func() {
		_ = ret.Close()
	})

	ret.populate(t)
	return ret
}

// reopen closes and opens the database, as at a restart.
func (d *activityDB) reopen(t *testing.T) {
	t.Helper()
	require.NoError(t, d.Close())
	require.NoError(t, d.Open(d.path))
}

func (d *activityDB) do(t *testing.T, ctx context.Context, fn func(ctx context.Context) error) {
	t.Helper()
	require.NoError(t, txn.WithTxn(ctx, d.Database, fn))
}

func (d *activityDB) exec(t *testing.T, stmts ...string) {
	t.Helper()
	d.do(t, context.Background(), func(ctx context.Context) error {
		for _, stmt := range stmts {
			if _, _, err := d.ExecSQL(ctx, stmt, nil); err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *activityDB) queryInt(t *testing.T, query string) int {
	t.Helper()
	var ret int
	d.do(t, context.Background(), func(ctx context.Context) error {
		_, rows, err := d.QuerySQL(ctx, query, nil)
		if err != nil {
			return err
		}
		require.Len(t, rows, 1)
		v, ok := rows[0][0].(int64)
		require.True(t, ok, "%v is not an integer", rows[0][0])
		ret = int(v)
		return nil
	})
	return ret
}

// populate creates three scenes and two images, all with the same performer
// and studio, and the scenes in the same group.
func (d *activityDB) populate(t *testing.T) {
	d.do(t, context.Background(), func(ctx context.Context) error {
		performer := &models.Performer{Name: "performer"}
		if err := d.Performer.Create(ctx, &models.CreatePerformerInput{Performer: performer}); err != nil {
			return err
		}
		d.performer = performer.ID

		studio := &models.Studio{Name: "studio"}
		if err := d.Studio.Create(ctx, &models.CreateStudioInput{Studio: studio}); err != nil {
			return err
		}
		d.studio = studio.ID

		group := &models.Group{Name: "group"}
		if err := d.Group.Create(ctx, group); err != nil {
			return err
		}
		d.group = group.ID

		for i := 0; i < 3; i++ {
			scene := &models.Scene{
				Title:        fmt.Sprintf("scene %d", i),
				StudioID:     &d.studio,
				PerformerIDs: models.NewRelatedIDs([]int{d.performer}),
				Groups:       models.NewRelatedGroups([]models.GroupsScenes{{GroupID: d.group}}),
			}
			if err := d.Scene.Create(ctx, scene, nil); err != nil {
				return err
			}
			d.scenes = append(d.scenes, scene.ID)
		}

		for i := 0; i < 2; i++ {
			image := &models.Image{
				Title:        fmt.Sprintf("image %d", i),
				StudioID:     &d.studio,
				PerformerIDs: models.NewRelatedIDs([]int{d.performer}),
			}
			if err := d.Image.Create(ctx, &models.CreateImageInput{Image: image}); err != nil {
				return err
			}
			d.images = append(d.images, image.ID)
		}

		return nil
	})
}

func asUser(name string, roleID string) context.Context {
	return models.SetCurrentUser(context.Background(), userOfRole(name, roleID))
}

// activityDates returns n distinct dates, starting at the hour.
func activityDates(hour int, n int) []time.Time {
	var ret []time.Time
	for i := 0; i < n; i++ {
		ret = append(ret, time.Date(2024, 1, 1, hour+i, 0, 0, 0, time.UTC))
	}
	return ret
}

// activity is the activity of a user as the queries see it.
type activity struct {
	views        []int
	oCounts      []int
	resumeTimes  []float64
	durations    []float64
	imageOCounts []int

	totalViews       int
	uniqueViews      int
	totalO           int
	totalImageO      int
	totalDuration    float64
	performerO       int
	studioO          int
	groupO           int
	performerMatches bool
}

func (d *activityDB) activityOf(t *testing.T, ctx context.Context) activity {
	t.Helper()

	var ret activity
	d.do(t, ctx, func(ctx context.Context) error {
		for _, id := range d.scenes {
			views, err := d.Scene.CountViews(ctx, id)
			require.NoError(t, err)
			oCount, err := d.Scene.GetOCount(ctx, id)
			require.NoError(t, err)
			scene, err := d.Scene.Find(ctx, id)
			require.NoError(t, err)

			ret.views = append(ret.views, views)
			ret.oCounts = append(ret.oCounts, oCount)
			ret.resumeTimes = append(ret.resumeTimes, scene.ResumeTime)
			ret.durations = append(ret.durations, scene.PlayDuration)
		}

		for _, id := range d.images {
			image, err := d.Image.Find(ctx, id)
			require.NoError(t, err)
			ret.imageOCounts = append(ret.imageOCounts, image.OCounter)
		}

		var err error
		ret.totalViews, err = d.Scene.CountAllViews(ctx)
		require.NoError(t, err)
		ret.uniqueViews, err = d.Scene.CountUniqueViews(ctx)
		require.NoError(t, err)
		ret.totalO, err = d.Scene.GetAllOCount(ctx)
		require.NoError(t, err)
		ret.totalImageO, err = d.Image.OCount(ctx)
		require.NoError(t, err)
		ret.totalDuration, err = d.Scene.PlayDuration(ctx)
		require.NoError(t, err)

		sceneO, err := d.Scene.OCountByPerformerID(ctx, d.performer)
		require.NoError(t, err)
		imageO, err := d.Image.OCountByPerformerID(ctx, d.performer)
		require.NoError(t, err)
		ret.performerO = sceneO + imageO

		sceneO, err = d.Scene.OCountByStudioID(ctx, d.studio, 0)
		require.NoError(t, err)
		imageO, err = d.Image.OCountByStudioID(ctx, d.studio, 0)
		require.NoError(t, err)
		ret.studioO = sceneO + imageO

		ret.groupO, err = d.Scene.OCountByGroupID(ctx, d.group)
		require.NoError(t, err)

		// the performer filter on the o-counter derived from the activity
		performers, _, err := d.Performer.Query(ctx, &models.PerformerFilterType{
			OCounter: &models.IntCriterionInput{Value: ret.performerO, Modifier: models.CriterionModifierEquals},
		}, nil)
		require.NoError(t, err)
		ret.performerMatches = len(performers) == 1

		return nil
	})

	return ret
}

func (d *activityDB) sceneIDs(t *testing.T, ctx context.Context, filter *models.SceneFilterType, sort string) []int {
	t.Helper()

	var findFilter *models.FindFilterType
	if sort != "" {
		direction := models.SortDirectionEnumDesc
		findFilter = &models.FindFilterType{Sort: &sort, Direction: &direction}
	}

	var ret []int
	d.do(t, ctx, func(ctx context.Context) error {
		result, err := d.Scene.Query(ctx, models.SceneQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: findFilter},
			SceneFilter:  filter,
		})
		if err != nil {
			return err
		}
		ret = result.IDs
		return nil
	})
	return ret
}

func (d *activityDB) imageIDs(t *testing.T, ctx context.Context, filter *models.ImageFilterType, sort string) []int {
	t.Helper()

	var findFilter *models.FindFilterType
	if sort != "" {
		direction := models.SortDirectionEnumDesc
		findFilter = &models.FindFilterType{Sort: &sort, Direction: &direction}
	}

	var ret []int
	d.do(t, ctx, func(ctx context.Context) error {
		result, err := d.Image.Query(ctx, models.ImageQueryOptions{
			QueryOptions: models.QueryOptions{FindFilter: findFilter},
			ImageFilter:  filter,
		})
		if err != nil {
			return err
		}
		ret = result.IDs
		return nil
	})
	return ret
}

// record records views, o dates and a resume point of a scene, and o counts
// of an image. The dates of different scenes differ.
func (d *activityDB) record(t *testing.T, ctx context.Context, scene int, views int, os int, resumeTime float64, duration float64, image int, imageOs int) {
	t.Helper()

	d.do(t, ctx, func(ctx context.Context) error {
		if views > 0 {
			if _, err := d.Scene.AddViews(ctx, d.scenes[scene], activityDates(scene*5, views)); err != nil {
				return err
			}
		}
		if os > 0 {
			if _, err := d.Scene.AddO(ctx, d.scenes[scene], activityDates(16+scene*3, os)); err != nil {
				return err
			}
		}
		if resumeTime > 0 || duration > 0 {
			if _, err := d.Scene.SaveActivity(ctx, d.scenes[scene], &resumeTime, &duration); err != nil {
				return err
			}
		}
		for i := 0; i < imageOs; i++ {
			if _, err := d.Image.IncrementOCounter(ctx, d.images[image]); err != nil {
				return err
			}
		}
		return nil
	})
}

func gt(v int) *models.IntCriterionInput {
	return &models.IntCriterionInput{Value: v, Modifier: models.CriterionModifierGreaterThan}
}

// TestUserActivitySeparate tests that each user records and sees their own
// activity, in the counts, filters, sorts and statistics.
func TestUserActivitySeparate(t *testing.T) {
	d := newActivityDB(t)
	d.admin = "admin"
	require.NoError(t, d.AdoptLegacyActivity(context.Background()))

	alice := asUser("alice", models.RoleIDEditor)
	// a viewer records their own activity
	bob := asUser("bob", models.RoleIDViewer)

	d.record(t, alice, 0, 2, 1, 10, 100, 0, 2)
	d.record(t, alice, 1, 1, 0, 0, 0, 0, 0)
	d.record(t, bob, 1, 4, 0, 20, 50, 1, 3)
	d.record(t, bob, 2, 0, 2, 0, 0, 0, 0)

	assert.Equal(t, activity{
		views:            []int{2, 1, 0},
		oCounts:          []int{1, 0, 0},
		resumeTimes:      []float64{10, 0, 0},
		durations:        []float64{100, 0, 0},
		imageOCounts:     []int{2, 0},
		totalViews:       3,
		uniqueViews:      2,
		totalO:           1,
		totalImageO:      2,
		totalDuration:    100,
		performerO:       3,
		studioO:          3,
		groupO:           1,
		performerMatches: true,
	}, d.activityOf(t, alice), "alice")

	assert.Equal(t, activity{
		views:            []int{0, 4, 0},
		oCounts:          []int{0, 0, 2},
		resumeTimes:      []float64{0, 20, 0},
		durations:        []float64{0, 50, 0},
		imageOCounts:     []int{0, 3},
		totalViews:       4,
		uniqueViews:      1,
		totalO:           2,
		totalImageO:      3,
		totalDuration:    50,
		performerO:       5,
		studioO:          5,
		groupO:           2,
		performerMatches: true,
	}, d.activityOf(t, bob), "bob")

	// the admin recorded nothing
	admin := d.activityOf(t, asUser("admin", models.RoleIDAdmin))
	assert.Equal(t, []int{0, 0, 0}, admin.views, "admin")
	assert.Equal(t, 0, admin.totalImageO, "admin")

	s := d.scenes
	i := d.images

	// filters
	assert.Equal(t, []int{s[0]}, d.sceneIDs(t, alice, &models.SceneFilterType{PlayCount: gt(1)}, ""), "alice play count")
	assert.Equal(t, []int{s[1]}, d.sceneIDs(t, bob, &models.SceneFilterType{PlayCount: gt(1)}, ""), "bob play count")
	assert.Equal(t, []int{s[0]}, d.sceneIDs(t, alice, &models.SceneFilterType{OCounter: gt(0)}, ""), "alice o-counter")
	assert.Equal(t, []int{s[2]}, d.sceneIDs(t, bob, &models.SceneFilterType{OCounter: gt(0)}, ""), "bob o-counter")
	assert.Equal(t, []int{s[0]}, d.sceneIDs(t, alice, &models.SceneFilterType{ResumeTime: gt(0)}, ""), "alice resume time")
	assert.Equal(t, []int{s[1]}, d.sceneIDs(t, bob, &models.SceneFilterType{ResumeTime: gt(0)}, ""), "bob resume time")
	assert.Equal(t, []int{s[0]}, d.sceneIDs(t, alice, &models.SceneFilterType{PlayDuration: gt(60)}, ""), "alice play duration")
	assert.Empty(t, d.sceneIDs(t, bob, &models.SceneFilterType{PlayDuration: gt(60)}, ""), "bob play duration")
	lastPlayed := &models.TimestampCriterionInput{Value: "2000-01-01", Modifier: models.CriterionModifierGreaterThan}
	assert.ElementsMatch(t, []int{s[0], s[1]}, d.sceneIDs(t, alice, &models.SceneFilterType{LastPlayedAt: lastPlayed}, ""), "alice last played")
	assert.ElementsMatch(t, []int{s[1]}, d.sceneIDs(t, bob, &models.SceneFilterType{LastPlayedAt: lastPlayed}, ""), "bob last played")
	assert.Equal(t, []int{i[0]}, d.imageIDs(t, alice, &models.ImageFilterType{OCounter: gt(0)}, ""), "alice image o-counter")
	assert.Equal(t, []int{i[1]}, d.imageIDs(t, bob, &models.ImageFilterType{OCounter: gt(0)}, ""), "bob image o-counter")

	// sorts
	first := func(ids []int) int {
		require.NotEmpty(t, ids)
		return ids[0]
	}
	assert.Equal(t, s[0], first(d.sceneIDs(t, alice, nil, "play_count")), "alice play count sort")
	assert.Equal(t, s[1], first(d.sceneIDs(t, bob, nil, "play_count")), "bob play count sort")
	assert.Equal(t, s[0], first(d.sceneIDs(t, alice, nil, "o_counter")), "alice o-counter sort")
	assert.Equal(t, s[2], first(d.sceneIDs(t, bob, nil, "o_counter")), "bob o-counter sort")
	assert.Equal(t, s[0], first(d.sceneIDs(t, alice, nil, "resume_time")), "alice resume time sort")
	assert.Equal(t, s[1], first(d.sceneIDs(t, bob, nil, "resume_time")), "bob resume time sort")
	assert.Equal(t, s[0], first(d.sceneIDs(t, alice, nil, "play_duration")), "alice play duration sort")
	assert.Equal(t, s[1], first(d.sceneIDs(t, bob, nil, "play_duration")), "bob play duration sort")
	assert.Equal(t, s[0], first(d.sceneIDs(t, alice, nil, "last_o_at")), "alice last o sort")
	assert.Equal(t, s[2], first(d.sceneIDs(t, bob, nil, "last_o_at")), "bob last o sort")
	assert.Equal(t, i[0], first(d.imageIDs(t, alice, nil, "o_counter")), "alice image o-counter sort")
	assert.Equal(t, i[1], first(d.imageIDs(t, bob, nil, "o_counter")), "bob image o-counter sort")

	// resetting is personal too
	d.do(t, bob, func(ctx context.Context) error {
		if _, err := d.Scene.ResetActivity(ctx, s[1], true, true); err != nil {
			return err
		}
		if _, err := d.Scene.DeleteAllViews(ctx, s[1]); err != nil {
			return err
		}
		_, err := d.Image.ResetOCounter(ctx, i[1])
		return err
	})
	bobAfter := d.activityOf(t, bob)
	assert.Equal(t, []int{0, 0, 0}, bobAfter.views, "bob after reset")
	assert.Equal(t, []float64{0, 0, 0}, bobAfter.resumeTimes, "bob after reset")
	assert.Equal(t, []int{0, 0}, bobAfter.imageOCounts, "bob after reset")
	assert.Equal(t, []int{2, 1, 0}, d.activityOf(t, alice).views, "alice after bob reset")
}

// TestUserActivityQuotedUserName tests the filters and sorts on the activity of
// a user whose name is not a valid SQL literal.
func TestUserActivityQuotedUserName(t *testing.T) {
	d := newActivityDB(t)
	d.admin = "admin"
	require.NoError(t, d.AdoptLegacyActivity(context.Background()))

	quoted := asUser("o'brien\\", models.RoleIDEditor)
	d.record(t, quoted, 1, 2, 1, 10, 100, 1, 2)
	d.record(t, asUser("alice", models.RoleIDEditor), 0, 3, 2, 20, 200, 0, 3)

	a := d.activityOf(t, quoted)
	assert.Equal(t, []int{0, 2, 0}, a.views, "views")
	assert.Equal(t, []int{0, 2}, a.imageOCounts, "image o counts")
	assert.Equal(t, 3, a.studioO, "studio o count")
	assert.True(t, a.performerMatches, "performer o count filter")

	s := d.scenes
	assert.Equal(t, []int{s[1]}, d.sceneIDs(t, quoted, &models.SceneFilterType{PlayCount: gt(1)}, ""), "play count filter")
	assert.Equal(t, []int{s[1]}, d.sceneIDs(t, quoted, &models.SceneFilterType{ResumeTime: gt(0)}, ""), "resume time filter")
	for _, sort := range []string{"play_count", "last_played_at", "last_o_at", "o_counter", "resume_time"} {
		ids := d.sceneIDs(t, quoted, nil, sort)
		require.NotEmpty(t, ids, sort)
		assert.Equal(t, s[1], ids[0], sort)
	}
	assert.Equal(t, d.images[1], d.imageIDs(t, quoted, nil, "o_counter")[0], "image o-counter sort")

	d.do(t, quoted, func(ctx context.Context) error {
		for _, sort := range []string{"o_counter", "play_count", "last_o_at", "last_played_at"} {
			if _, _, err := d.Performer.Query(ctx, nil, &models.FindFilterType{Sort: &sort}); err != nil {
				return fmt.Errorf("performer %s sort: %w", sort, err)
			}
		}
		for _, sort := range []string{"o_counter", "o_counter_all"} {
			if _, _, err := d.Studio.Query(ctx, nil, &models.FindFilterType{Sort: &sort}); err != nil {
				return fmt.Errorf("studio %s sort: %w", sort, err)
			}
		}
		sort := "o_counter"
		_, _, err := d.Group.Query(ctx, &models.GroupFilterType{OCounter: gt(0)}, &models.FindFilterType{Sort: &sort})
		return err
	})
}

// TestUserActivityVisibility tests that the activity on scenes that are
// hidden from the user is not counted.
func TestUserActivityVisibility(t *testing.T) {
	d := newActivityDB(t)
	d.admin = "admin"
	require.NoError(t, d.AdoptLegacyActivity(context.Background()))

	bob := asUser("bob", models.RoleIDViewer)
	d.record(t, bob, 1, 4, 0, 0, 0, 1, 3)
	d.record(t, bob, 2, 1, 2, 0, 0, 0, 0)

	// hide scene 2 from viewers
	d.do(t, context.Background(), func(ctx context.Context) error {
		restricted := models.SceneVisibilityRestricted
		_, err := d.Scene.UpdatePartial(ctx, d.scenes[2], models.ScenePartial{Visibility: &restricted})
		return err
	})

	d.do(t, bob, func(ctx context.Context) error {
		total, err := d.Scene.CountAllViews(ctx)
		require.NoError(t, err)
		assert.Equal(t, 4, total, "views")

		totalO, err := d.Scene.GetAllOCount(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, totalO, "o count")

		sceneO, err := d.Scene.OCountByPerformerID(ctx, d.performer)
		require.NoError(t, err)
		assert.Equal(t, 0, sceneO, "performer scene o count")

		// the performer filter counts the image o-counter only
		performers, _, err := d.Performer.Query(ctx, &models.PerformerFilterType{
			OCounter: &models.IntCriterionInput{Value: 3, Modifier: models.CriterionModifierEquals},
		}, nil)
		require.NoError(t, err)
		assert.Len(t, performers, 1, "performer o-counter filter")

		performers, _, err = d.Performer.Query(ctx, &models.PerformerFilterType{
			PlayCount: &models.IntCriterionInput{Value: 4, Modifier: models.CriterionModifierEquals},
		}, nil)
		require.NoError(t, err)
		assert.Len(t, performers, 1, "performer play count filter")

		groups, _, err := d.Group.Query(ctx, &models.GroupFilterType{OCounter: gt(0)}, nil)
		require.NoError(t, err)
		assert.Empty(t, groups, "group o-counter filter")
		return nil
	})
}

// TestUserActivityNoUser tests that the activity of the first admin is used
// when there is no user in the context.
func TestUserActivityNoUser(t *testing.T) {
	d := newActivityDB(t)
	d.admin = "admin"
	require.NoError(t, d.AdoptLegacyActivity(context.Background()))

	admin := asUser("admin", models.RoleIDAdmin)
	alice := asUser("alice", models.RoleIDEditor)

	// a background task, and a DLNA request
	d.record(t, context.Background(), 0, 1, 1, 5, 5, 0, 1)
	d.record(t, models.SetPublicScenesOnly(context.Background()), 0, 1, 0, 0, 0, 0, 0)
	// export and import use the activity of the first admin
	d.record(t, models.WithFirstAdminActivity(alice), 0, 1, 0, 0, 0, 0, 0)
	// a plugin hook that no user triggered
	d.record(t, asUser("", models.RoleIDAdmin), 0, 1, 0, 0, 0, 0, 0)

	a := d.activityOf(t, admin)
	assert.Equal(t, []int{4, 0, 0}, a.views, "admin views")
	assert.Equal(t, []int{1, 0, 0}, a.oCounts, "admin o counts")
	assert.Equal(t, []float64{5, 0, 0}, a.resumeTimes, "admin resume times")
	assert.Equal(t, []int{1, 0}, a.imageOCounts, "admin image o counts")

	assert.Equal(t, a, d.activityOf(t, context.Background()), "no user")
	assert.Equal(t, []int{0, 0, 0}, d.activityOf(t, alice).views, "alice")

	// the first admin changes with the users
	d.admin = "alice"
	assert.Equal(t, []int{0, 0, 0}, d.activityOf(t, context.Background()).views, "no user, alice is the first admin")
}

// TestUserActivityAdoption tests that the legacy activity is copied to the
// first admin exactly once, and waits while no admin exists.
func TestUserActivityAdoption(t *testing.T) {
	d := newActivityDB(t)
	s := d.scenes
	i := d.images

	// activity in the legacy tables and columns
	d.exec(t,
		fmt.Sprintf("INSERT INTO scenes_view_dates (scene_id, view_date) VALUES (%d, '2024-01-01T00:00:00Z'), (%d, '2024-01-01T01:00:00Z'), (%d, '2024-01-01T02:00:00Z')", s[0], s[0], s[1]),
		fmt.Sprintf("INSERT INTO scenes_o_dates (scene_id, o_date) VALUES (%d, '2024-01-01T03:00:00Z')", s[2]),
		fmt.Sprintf("UPDATE scenes SET resume_time = 30, play_duration = 300 WHERE id = %d", s[1]),
		fmt.Sprintf("UPDATE images SET o_counter = 4 WHERE id = %d", i[1]),
		"DROP TABLE user_scene_views",
		"DROP TABLE user_scene_o_dates",
		"DROP TABLE user_scene_activity",
		"DROP TABLE user_image_o_counters",
		"DROP TABLE user_activity_adoption",
	)

	legacy := activity{
		views:         []int{2, 1, 0},
		oCounts:       []int{0, 0, 1},
		resumeTimes:   []float64{0, 30, 0},
		durations:     []float64{0, 300, 0},
		imageOCounts:  []int{0, 4},
		totalViews:    3,
		uniqueViews:   2,
		totalO:        1,
		totalImageO:   4,
		totalDuration: 300,
		performerO:    5,
		studioO:       5,
		groupO:        1,

		performerMatches: true,
	}
	none := activity{
		views:            []int{0, 0, 0},
		oCounts:          []int{0, 0, 0},
		resumeTimes:      []float64{0, 0, 0},
		durations:        []float64{0, 0, 0},
		imageOCounts:     []int{0, 0},
		performerMatches: true,
	}

	// the tables are created and the legacy activity is copied at the
	// restart, but no admin exists to adopt it
	d.reopen(t)
	assert.Equal(t, 0, d.queryInt(t, "SELECT COUNT(*) FROM user_activity_adoption WHERE user_name IS NOT NULL"), "adopted without admin")
	assert.Equal(t, legacy, d.activityOf(t, context.Background()), "without users")

	// an admin exists, but did not adopt the activity yet: the admin uses the
	// legacy activity, other users have their own
	d.admin = "admin"
	assert.Equal(t, legacy, d.activityOf(t, asUser("admin", models.RoleIDAdmin)), "admin before adoption")
	assert.Equal(t, none, d.activityOf(t, asUser("bob", models.RoleIDViewer)), "bob before adoption")

	require.NoError(t, d.AdoptLegacyActivity(context.Background()))
	assert.Equal(t, legacy, d.activityOf(t, asUser("admin", models.RoleIDAdmin)), "admin after adoption")
	assert.Equal(t, none, d.activityOf(t, asUser("bob", models.RoleIDViewer)), "bob after adoption")
	assert.Equal(t, 1, d.queryInt(t, "SELECT COUNT(*) FROM user_activity_adoption WHERE user_name = 'admin'"), "adoption record")
	assert.Equal(t, 0, d.queryInt(t, "SELECT (SELECT COUNT(*) FROM user_scene_views WHERE user_name = '') + (SELECT COUNT(*) FROM user_image_o_counters WHERE user_name = '')"), "unowned activity")

	// it runs only once: the adoption, and the copy at the next start
	require.NoError(t, d.AdoptLegacyActivity(context.Background()))
	d.exec(t, fmt.Sprintf("INSERT INTO scenes_view_dates (scene_id, view_date) VALUES (%d, '2024-01-02T00:00:00Z')", s[2]))
	d.reopen(t)
	assert.Equal(t, legacy, d.activityOf(t, asUser("admin", models.RoleIDAdmin)), "admin after restart")

	// the legacy tables and columns are not written
	d.record(t, asUser("admin", models.RoleIDAdmin), 0, 1, 1, 1, 1, 0, 1)
	assert.Equal(t, 4, d.queryInt(t, "SELECT COUNT(*) FROM scenes_view_dates"), "legacy views")
	assert.Equal(t, 1, d.queryInt(t, "SELECT COUNT(*) FROM scenes_o_dates"), "legacy o dates")
	assert.Equal(t, 0, d.queryInt(t, fmt.Sprintf("SELECT CAST(resume_time AS INTEGER) FROM scenes WHERE id = %d", s[0])), "legacy resume time")
	assert.Equal(t, 0, d.queryInt(t, fmt.Sprintf("SELECT o_counter FROM images WHERE id = %d", i[0])), "legacy image o-counter")
}

func TestUserActivityAnonymise(t *testing.T) {
	d := newActivityDB(t)
	d.admin = "admin"
	require.NoError(t, d.AdoptLegacyActivity(context.Background()))
	d.record(t, asUser("alice", models.RoleIDEditor), 0, 1, 1, 10, 10, 0, 1)

	out := filepath.Join(t.TempDir(), "anonymised.sqlite")
	anonymiser, err := sqlite.NewAnonymiser(d.Database, out)
	require.NoError(t, err)
	require.NoError(t, anonymiser.Anonymise(context.Background()))

	anonymised := sqlite.NewDatabase()
	require.NoError(t, anonymised.Open(out))
	t.Cleanup(func() {
		_ = anonymised.Close()
	})

	require.NoError(t, txn.WithReadTxn(context.Background(), anonymised, func(ctx context.Context) error {
		_, rows, err := anonymised.QuerySQL(ctx, `SELECT user_name FROM user_activity_adoption
			UNION ALL SELECT user_name FROM user_scene_views
			UNION ALL SELECT user_name FROM user_scene_o_dates
			UNION ALL SELECT user_name FROM user_scene_activity
			UNION ALL SELECT user_name FROM user_image_o_counters`, nil)
		require.NoError(t, err)
		require.NotEmpty(t, rows)
		for _, row := range rows {
			assert.IsType(t, "", row[0])
			assert.NotContains(t, []interface{}{"admin", "alice"}, row[0])
		}
		return nil
	}))
}

// TestUserActivityRecordedWithoutAdmin tests that the activity recorded while
// no admin exists is adopted by the first admin.
func TestUserActivityRecordedWithoutAdmin(t *testing.T) {
	d := newActivityDB(t)

	d.record(t, context.Background(), 0, 2, 0, 0, 0, 0, 1)

	d.admin = "admin"
	require.NoError(t, d.AdoptLegacyActivity(context.Background()))

	assert.Equal(t, []int{2, 0, 0}, d.activityOf(t, context.Background()).views, "no user")
	assert.Equal(t, []int{1, 0}, d.activityOf(t, asUser("admin", models.RoleIDAdmin)).imageOCounts, "admin")
	assert.Equal(t, []int{0, 0, 0}, d.activityOf(t, asUser("bob", models.RoleIDViewer)).views, "bob")
}

// TestUserActivityRenameAndRemove tests that the activity of a renamed user
// moves to the new name, and that the activity of a removed user is deleted.
func TestUserActivityRenameAndRemove(t *testing.T) {
	d := newActivityDB(t)
	d.admin = "admin"
	require.NoError(t, d.AdoptLegacyActivity(context.Background()))

	alice := asUser("alice", models.RoleIDEditor)
	bob := asUser("bob", models.RoleIDViewer)
	d.record(t, alice, 0, 2, 1, 10, 100, 0, 2)
	d.record(t, bob, 1, 1, 1, 20, 50, 1, 1)
	d.record(t, context.Background(), 2, 1, 0, 0, 0, 0, 0)

	aliceBefore := d.activityOf(t, alice)
	bobBefore := d.activityOf(t, bob)
	adminBefore := d.activityOf(t, context.Background())

	newName := "alicia"
	require.NoError(t, d.MoveUserActivity(context.Background(), "alice", &newName))
	assert.Equal(t, aliceBefore, d.activityOf(t, asUser("alicia", models.RoleIDEditor)), "renamed")
	assert.Equal(t, []int{0, 0, 0}, d.activityOf(t, alice).views, "old name")

	require.NoError(t, d.MoveUserActivity(context.Background(), "bob", nil))
	assert.Equal(t, []int{0, 0, 0}, d.activityOf(t, bob).views, "removed")
	assert.Equal(t, 0, d.queryInt(t, "SELECT (SELECT COUNT(*) FROM user_scene_views WHERE user_name = 'bob') + (SELECT COUNT(*) FROM user_scene_o_dates WHERE user_name = 'bob') + (SELECT COUNT(*) FROM user_scene_activity WHERE user_name = 'bob') + (SELECT COUNT(*) FROM user_image_o_counters WHERE user_name = 'bob')"), "rows of the removed user")
	assert.NotEqual(t, bobBefore.views, d.activityOf(t, bob).views)

	// the other users keep their activity
	assert.Equal(t, adminBefore, d.activityOf(t, context.Background()), "admin")
	assert.Equal(t, aliceBefore, d.activityOf(t, asUser("alicia", models.RoleIDEditor)), "alicia")
}

// TestUserActivityCopy tests that copying the activity of a user to a new user
// keeps the activity of the source.
func TestUserActivityCopy(t *testing.T) {
	d := newActivityDB(t)
	d.admin = "admin"
	require.NoError(t, d.AdoptLegacyActivity(context.Background()))

	alice := asUser("alice", models.RoleIDEditor)
	d.record(t, alice, 0, 2, 1, 10, 100, 0, 2)
	d.record(t, asUser("bob", models.RoleIDViewer), 1, 1, 1, 20, 50, 1, 1)

	aliceBefore := d.activityOf(t, alice)

	require.NoError(t, d.CopyUserActivity(context.Background(), "alice", "carol"))
	// equal to alice, so without the activity of bob
	assert.Equal(t, aliceBefore, d.activityOf(t, asUser("carol", models.RoleIDViewer)), "copy")
	assert.Equal(t, aliceBefore, d.activityOf(t, alice), "source")
}

type mergeConfig struct{}

func (mergeConfig) GetVideoFileNamingAlgorithm() models.HashAlgorithm {
	return models.HashAlgorithmOshash
}

// merge merges the source scenes into the destination scene as the user, as
// the sceneMerge mutation does.
func (d *activityDB) merge(t *testing.T, ctx context.Context, sources []int, destination int, options scene.MergeOptions) {
	t.Helper()

	service := &scene.Service{
		File:             d.File,
		Repository:       d.Scene,
		MarkerRepository: d.SceneMarker,
		Config:           mergeConfig{},
	}
	fileDeleter := &scene.FileDeleter{FileNamingAlgo: models.HashAlgorithmOshash}

	d.do(t, ctx, func(ctx context.Context) error {
		return service.Merge(ctx, sources, destination, fileDeleter, options)
	})
}

func TestUserActivityFullUpdate(t *testing.T) {
	d := newActivityDB(t)
	d.admin = "admin"
	require.NoError(t, d.AdoptLegacyActivity(context.Background()))
	alice := asUser("alice", models.RoleIDEditor)

	update := func(resumeTime float64, imageO int) {
		d.do(t, alice, func(ctx context.Context) error {
			s, err := d.Scene.Find(ctx, d.scenes[0])
			if err != nil {
				return err
			}
			s.ResumeTime = resumeTime
			if err := d.Scene.Update(ctx, s); err != nil {
				return err
			}

			i, err := d.Image.Find(ctx, d.images[0])
			if err != nil {
				return err
			}
			i.OCounter = imageO
			return d.Image.Update(ctx, i)
		})
	}
	rows := func() int {
		return d.queryInt(t, "SELECT (SELECT COUNT(*) FROM user_scene_activity) + (SELECT COUNT(*) FROM user_image_o_counters)")
	}

	update(0, 0)
	assert.Equal(t, 0, rows(), "rows after an update without activity")

	update(10, 2)
	assert.Equal(t, 2, rows(), "rows after an update with activity")
	assert.Equal(t, []float64{10, 0, 0}, d.activityOf(t, alice).resumeTimes)
	assert.Equal(t, []int{2, 0}, d.activityOf(t, alice).imageOCounts)

	update(0, 0)
	assert.Equal(t, []float64{0, 0, 0}, d.activityOf(t, alice).resumeTimes)
	assert.Equal(t, []int{0, 0}, d.activityOf(t, alice).imageOCounts)
}

// TestUserActivitySceneMerge tests that a scene merge moves the activity of
// every user to the destination scene, without mixing the users.
func TestUserActivitySceneMerge(t *testing.T) {
	d := newActivityDB(t)
	d.admin = "admin"
	require.NoError(t, d.AdoptLegacyActivity(context.Background()))

	admin := asUser("admin", models.RoleIDAdmin)
	alice := asUser("alice", models.RoleIDEditor)
	bob := asUser("bob", models.RoleIDViewer)
	s := d.scenes

	// scene 0 is the destination, scenes 1 and 2 are merged into it
	d.record(t, alice, 0, 1, 0, 3, 10, 0, 0)
	d.record(t, alice, 1, 2, 1, 5, 100, 0, 0)
	d.record(t, bob, 1, 3, 2, 0, 40, 0, 0)
	d.record(t, admin, 1, 1, 0, 0, 20, 0, 0)
	d.record(t, alice, 2, 1, 1, 0, 7, 0, 0)

	// the merge dialog sets the play duration of the merging user to the sum
	// of their play durations, and keeps the play and o history
	partial := models.NewScenePartial()
	partial.PlayDuration = models.NewOptionalFloat64(20)
	d.merge(t, admin, []int{s[1]}, s[0], scene.MergeOptions{
		ScenePartial:       partial,
		IncludePlayHistory: true,
		IncludeOHistory:    true,
	})

	d.scenes = []int{s[0], s[2]}

	a := d.activityOf(t, alice)
	assert.Equal(t, []int{3, 1}, a.views, "alice views")
	assert.Equal(t, []int{1, 1}, a.oCounts, "alice o counts")
	// resume points are not moved, play durations are added
	assert.Equal(t, []float64{3, 0}, a.resumeTimes, "alice resume times")
	assert.Equal(t, []float64{110, 7}, a.durations, "alice play durations")

	b := d.activityOf(t, bob)
	assert.Equal(t, []int{3, 0}, b.views, "bob views")
	assert.Equal(t, []int{2, 0}, b.oCounts, "bob o counts")
	assert.Equal(t, []float64{40, 0}, b.durations, "bob play durations")

	// the merging user has the play duration that the merge sets
	ad := d.activityOf(t, admin)
	assert.Equal(t, []int{1, 0}, ad.views, "admin views")
	assert.Equal(t, []float64{20, 0}, ad.durations, "admin play duration")

	// nothing is left of the source scene
	assert.Equal(t, 0, d.queryInt(t, fmt.Sprintf("SELECT (SELECT COUNT(*) FROM user_scene_views WHERE scene_id = %[1]d) + (SELECT COUNT(*) FROM user_scene_o_dates WHERE scene_id = %[1]d) + (SELECT COUNT(*) FROM user_scene_activity WHERE scene_id = %[1]d)", s[1])), "activity of the source")

	// a merge that does not keep the history drops it with the source
	d.merge(t, admin, []int{s[2]}, s[0], scene.MergeOptions{ScenePartial: models.NewScenePartial()})
	d.scenes = []int{s[0]}

	a = d.activityOf(t, alice)
	assert.Equal(t, []int{3}, a.views, "alice views after the second merge")
	assert.Equal(t, []int{1}, a.oCounts, "alice o counts after the second merge")
	assert.Equal(t, []float64{110}, a.durations, "alice play durations after the second merge")
	assert.Equal(t, 0, d.queryInt(t, fmt.Sprintf("SELECT (SELECT COUNT(*) FROM user_scene_views WHERE scene_id = %[1]d) + (SELECT COUNT(*) FROM user_scene_o_dates WHERE scene_id = %[1]d) + (SELECT COUNT(*) FROM user_scene_activity WHERE scene_id = %[1]d)", s[2])), "activity of the second source")
}
