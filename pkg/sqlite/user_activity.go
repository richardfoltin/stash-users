package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/jmoiron/sqlx"

	"github.com/stashapp/stash/pkg/logger"
	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
)

// The viewing activity is stored per user. The tables are created when the
// database is opened, and the legacy activity is copied into them once, as
// unowned activity that the first admin adopts.
const (
	userSceneViewsTable       = "user_scene_views"
	userSceneODatesTable      = "user_scene_o_dates"
	userSceneActivityTable    = "user_scene_activity"
	userImageOCountersTable   = "user_image_o_counters"
	userActivityAdoptionTable = "user_activity_adoption"

	userNameColumn = "user_name"

	unownedActivity = ""
)

var userActivitySchema = []string{
	`CREATE TABLE IF NOT EXISTS user_scene_views (
		scene_id INTEGER NOT NULL REFERENCES scenes(id) ON DELETE CASCADE,
		user_name TEXT NOT NULL,
		view_date DATETIME NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS index_user_scene_views_on_scene_id ON user_scene_views (scene_id, user_name, view_date)`,
	`CREATE INDEX IF NOT EXISTS index_user_scene_views_on_user_name ON user_scene_views (user_name)`,
	`CREATE TABLE IF NOT EXISTS user_scene_o_dates (
		scene_id INTEGER NOT NULL REFERENCES scenes(id) ON DELETE CASCADE,
		user_name TEXT NOT NULL,
		o_date DATETIME NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS index_user_scene_o_dates_on_scene_id ON user_scene_o_dates (scene_id, user_name, o_date)`,
	`CREATE INDEX IF NOT EXISTS index_user_scene_o_dates_on_user_name ON user_scene_o_dates (user_name)`,
	`CREATE TABLE IF NOT EXISTS user_scene_activity (
		scene_id INTEGER NOT NULL REFERENCES scenes(id) ON DELETE CASCADE,
		user_name TEXT NOT NULL,
		resume_time REAL NOT NULL DEFAULT 0,
		play_duration REAL NOT NULL DEFAULT 0,
		PRIMARY KEY (scene_id, user_name)
	)`,
	`CREATE INDEX IF NOT EXISTS index_user_scene_activity_on_user_name ON user_scene_activity (user_name)`,
	`CREATE TABLE IF NOT EXISTS user_image_o_counters (
		image_id INTEGER NOT NULL REFERENCES images(id) ON DELETE CASCADE,
		user_name TEXT NOT NULL,
		o_counter INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (image_id, user_name)
	)`,
	`CREATE INDEX IF NOT EXISTS index_user_image_o_counters_on_user_name ON user_image_o_counters (user_name)`,
	`CREATE TABLE IF NOT EXISTS user_activity_adoption (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		copied_at DATETIME NOT NULL,
		user_name TEXT,
		adopted_at DATETIME
	)`,
	// rows deleted while foreign keys were disabled
	`DELETE FROM user_scene_views WHERE scene_id NOT IN (SELECT id FROM scenes)`,
	`DELETE FROM user_scene_o_dates WHERE scene_id NOT IN (SELECT id FROM scenes)`,
	`DELETE FROM user_scene_activity WHERE scene_id NOT IN (SELECT id FROM scenes)`,
	`DELETE FROM user_image_o_counters WHERE image_id NOT IN (SELECT id FROM images)`,
}

var legacyActivityCopy = []string{
	`INSERT INTO user_scene_views (scene_id, user_name, view_date)
		SELECT scene_id, '', view_date FROM scenes_view_dates`,
	`INSERT INTO user_scene_o_dates (scene_id, user_name, o_date)
		SELECT scene_id, '', o_date FROM scenes_o_dates`,
	`INSERT INTO user_scene_activity (scene_id, user_name, resume_time, play_duration)
		SELECT id, '', COALESCE(resume_time, 0), COALESCE(play_duration, 0) FROM scenes
		WHERE COALESCE(resume_time, 0) != 0 OR COALESCE(play_duration, 0) != 0`,
	`INSERT INTO user_image_o_counters (image_id, user_name, o_counter)
		SELECT id, '', o_counter FROM images WHERE COALESCE(o_counter, 0) != 0`,
	`INSERT INTO user_activity_adoption (id, copied_at) VALUES (1, CURRENT_TIMESTAMP)`,
}

type userActivityState struct {
	mu         sync.RWMutex
	firstAdmin func(ctx context.Context) string
	adopted    bool
}

func (s *userActivityState) get() (func(ctx context.Context) string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.firstAdmin, s.adopted
}

func (s *userActivityState) setAdopted(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adopted = v
}

// SetFirstAdmin sets the function that returns the name of the first admin.
func (db *Database) SetFirstAdmin(fn func(ctx context.Context) string) {
	db.userActivity.mu.Lock()
	defer db.userActivity.mu.Unlock()
	db.userActivity.firstAdmin = fn
}

type userActivityDatabaseKey struct{}

func (db *Database) withUserActivity(ctx context.Context) context.Context {
	return context.WithValue(ctx, userActivityDatabaseKey{}, db)
}

// activityUser returns the name of the user whose activity is read and
// written: the current user, or the first admin when there is no current user
// or the context is marked with models.WithFirstAdminActivity. The activity of
// the first admin is unowned until it is adopted.
func activityUser(ctx context.Context) string {
	db, _ := ctx.Value(userActivityDatabaseKey{}).(*Database)
	if db == nil {
		return unownedActivity
	}

	firstAdmin, adopted := db.userActivity.get()
	if firstAdmin == nil {
		return unownedActivity
	}

	var current string
	if u := models.GetCurrentUser(ctx); u != nil && !models.IsFirstAdminActivity(ctx) {
		current = u.Username
	}

	// avoids reading the users for every query of a logged in user
	if current != "" && adopted {
		return current
	}

	admin := firstAdmin(ctx)
	if admin == "" || (!adopted && (current == "" || current == admin)) {
		return unownedActivity
	}

	if current == "" {
		return admin
	}

	return current
}

func (db *Database) ensureUserActivity() error {
	for _, stmt := range userActivitySchema {
		if _, err := db.writeDB.Exec(stmt); err != nil {
			return fmt.Errorf("creating the user activity tables: %w", err)
		}
	}

	if err := copyLegacyActivity(db.writeDB); err != nil {
		return err
	}

	var adoptedBy sql.NullString
	if err := db.writeDB.Get(&adoptedBy, "SELECT user_name FROM "+userActivityAdoptionTable+" WHERE id = 1"); err != nil {
		return fmt.Errorf("reading the user activity adoption: %w", err)
	}
	db.userActivity.setAdopted(adoptedBy.Valid)

	// the adoption is retried when the users change
	if err := db.AdoptLegacyActivity(context.Background()); err != nil {
		logger.Warnf("error adopting the legacy viewing activity: %v", err)
	}

	return nil
}

func copyLegacyActivity(db *sqlx.DB) error {
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("copying the legacy activity: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var copied int
	if err := tx.Get(&copied, "SELECT COUNT(*) FROM "+userActivityAdoptionTable); err != nil {
		return fmt.Errorf("copying the legacy activity: %w", err)
	}

	if copied > 0 {
		return nil
	}

	for _, stmt := range legacyActivityCopy {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("copying the legacy activity: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("copying the legacy activity: %w", err)
	}

	logger.Info("Copied the existing viewing activity to the per-user activity tables")
	return nil
}

// AdoptLegacyActivity assigns the unowned activity to the first admin, once.
func (db *Database) AdoptLegacyActivity(ctx context.Context) error {
	firstAdmin, adopted := db.userActivity.get()
	if adopted || firstAdmin == nil {
		return nil
	}

	admin := firstAdmin(ctx)
	if admin == "" {
		return nil
	}

	moved := false
	if err := txn.WithTxn(ctx, db, func(ctx context.Context) error {
		var adoptedBy sql.NullString
		if err := dbWrapper.Get(ctx, &adoptedBy, "SELECT user_name FROM "+userActivityAdoptionTable+" WHERE id = 1"); err != nil {
			return err
		}

		if !adoptedBy.Valid {
			if err := moveUserActivity(ctx, unownedActivity, admin); err != nil {
				return err
			}

			if _, err := dbWrapper.Exec(ctx, "UPDATE "+userActivityAdoptionTable+" SET user_name = ?, adopted_at = CURRENT_TIMESTAMP WHERE id = 1", admin); err != nil {
				return err
			}

			moved = true
		}

		// set before the commit, so that the writers waiting for this
		// transaction use the name of the admin
		db.userActivity.setAdopted(true)
		return nil
	}); err != nil {
		db.userActivity.setAdopted(false)
		return fmt.Errorf("assigning the legacy activity to %s: %w", admin, err)
	}

	if moved {
		logger.Infof("Assigned the viewing activity recorded without users to %s", admin)
	}

	return nil
}

// MoveUserActivity adds the activity of the user from to the user to, or
// deletes it if to is nil.
func (db *Database) MoveUserActivity(ctx context.Context, from string, to *string) error {
	return txn.WithTxn(ctx, db, func(ctx context.Context) error {
		if to == nil {
			return deleteUserActivity(ctx, from)
		}

		return moveUserActivity(ctx, from, *to)
	})
}

var moveUserActivitySQL = []string{
	`UPDATE user_scene_views SET user_name = :to WHERE user_name = :from`,
	`UPDATE user_scene_o_dates SET user_name = :to WHERE user_name = :from`,
	`INSERT INTO user_scene_activity (scene_id, user_name, resume_time, play_duration)
		SELECT scene_id, :to, resume_time, play_duration FROM user_scene_activity WHERE user_name = :from
		ON CONFLICT (scene_id, user_name) DO UPDATE SET
			resume_time = CASE WHEN resume_time = 0 THEN excluded.resume_time ELSE resume_time END,
			play_duration = play_duration + excluded.play_duration`,
	`DELETE FROM user_scene_activity WHERE user_name = :from`,
	`INSERT INTO user_image_o_counters (image_id, user_name, o_counter)
		SELECT image_id, :to, o_counter FROM user_image_o_counters WHERE user_name = :from
		ON CONFLICT (image_id, user_name) DO UPDATE SET o_counter = o_counter + excluded.o_counter`,
	`DELETE FROM user_image_o_counters WHERE user_name = :from`,
}

func moveUserActivity(ctx context.Context, from string, to string) error {
	if from == to {
		return nil
	}

	arg := map[string]interface{}{"from": from, "to": to}
	for _, stmt := range moveUserActivitySQL {
		if _, err := dbWrapper.NamedExec(ctx, stmt, arg); err != nil {
			return fmt.Errorf("moving the activity of %q to %q: %w", from, to, err)
		}
	}

	return nil
}

func deleteUserActivity(ctx context.Context, name string) error {
	for _, table := range []string{userSceneViewsTable, userSceneODatesTable, userSceneActivityTable, userImageOCountersTable} {
		if _, err := dbWrapper.Exec(ctx, "DELETE FROM "+table+" WHERE user_name = ?", name); err != nil {
			return fmt.Errorf("deleting the activity of %q: %w", name, err)
		}
	}

	return nil
}

// MergeActivity moves the activity of every user from the source scenes to the
// destination. The merge sets the play duration of the activity user itself,
// so only those of the other users are added.
func (qb *SceneStore) MergeActivity(ctx context.Context, destinationID int, sourceIDs []int, options models.SceneActivityMergeOptions) error {
	if len(sourceIDs) == 0 {
		return nil
	}

	in := getInBinding(len(sourceIDs))
	args := []interface{}{destinationID}
	for _, id := range sourceIDs {
		args = append(args, id)
	}

	if options.PlayHistory {
		if _, err := dbWrapper.Exec(ctx, "UPDATE "+userSceneViewsTable+" SET scene_id = ? WHERE scene_id IN "+in, args...); err != nil {
			return fmt.Errorf("merging the play history: %w", err)
		}
	}

	if options.OHistory {
		if _, err := dbWrapper.Exec(ctx, "UPDATE "+userSceneODatesTable+" SET scene_id = ? WHERE scene_id IN "+in, args...); err != nil {
			return fmt.Errorf("merging the o history: %w", err)
		}
	}

	if options.PlayDuration {
		stmt := `INSERT INTO user_scene_activity (scene_id, user_name, resume_time, play_duration)
			SELECT ?, user_name, 0, SUM(play_duration) FROM user_scene_activity
			WHERE scene_id IN ` + in + ` AND user_name != ?
			GROUP BY user_name HAVING SUM(play_duration) != 0
			ON CONFLICT (scene_id, user_name) DO UPDATE SET play_duration = play_duration + excluded.play_duration`

		if _, err := dbWrapper.Exec(ctx, stmt, append(args, activityUser(ctx))...); err != nil {
			return fmt.Errorf("merging the play duration: %w", err)
		}
	}

	return nil
}

func userActivitySQL(ctx context.Context, table string, columns string) (string, []interface{}) {
	return fmt.Sprintf("(SELECT %s FROM %s WHERE %s = ?)", columns, table, userNameColumn), []interface{}{activityUser(ctx)}
}

func sceneViewsSQL(ctx context.Context) (string, []interface{}) {
	return userActivitySQL(ctx, userSceneViewsTable, sceneIDColumn+", "+sceneViewDateColumn)
}

func sceneODatesSQL(ctx context.Context) (string, []interface{}) {
	return userActivitySQL(ctx, userSceneODatesTable, sceneIDColumn+", "+sceneODateColumn)
}

func imageOCountersSQL(ctx context.Context) (string, []interface{}) {
	return userActivitySQL(ctx, userImageOCountersTable, "image_id AS id, o_counter")
}

func sceneActivitySQL(ctx context.Context, column string, sceneIDCol string) (string, []interface{}) {
	return fmt.Sprintf("COALESCE((SELECT activity.%s FROM %s AS activity WHERE activity.scene_id = %s AND activity.%s = ?), 0)",
		column, userSceneActivityTable, sceneIDCol, userNameColumn), []interface{}{activityUser(ctx)}
}

func imageOCounterSQL(ctx context.Context, imageIDCol string) (string, []interface{}) {
	return fmt.Sprintf("COALESCE((SELECT activity.o_counter FROM %s AS activity WHERE activity.image_id = %s AND activity.%s = ?), 0)",
		userImageOCountersTable, imageIDCol, userNameColumn), []interface{}{activityUser(ctx)}
}

// formatUserActivity formats the template with the image O counters as %[1]s,
// the scene O history as %[2]s, the play history as %[3]s and the visibility
// condition of sceneIDCol as %[4]s.
func formatUserActivity(ctx context.Context, template string, sceneIDCol string) (string, []interface{}) {
	imageOCounters, userArgs := imageOCountersSQL(ctx)
	sceneODates, _ := sceneODatesSQL(ctx)
	sceneViews, _ := sceneViewsSQL(ctx)

	// all fragments bind the same user name
	var args []interface{}
	for _, verb := range []string{"%[1]s", "%[2]s", "%[3]s"} {
		for i := 0; i < strings.Count(template, verb); i++ {
			args = append(args, userArgs...)
		}
	}

	return fmt.Sprintf(template, imageOCounters, sceneODates, sceneViews, andSceneVisible(ctx, sceneIDCol)), args
}

func sortByUserActivity(ctx context.Context, query *queryBuilder, template string, direction string) string {
	sql, args := formatUserActivity(ctx, template, "s."+sceneIDColumn)
	query.sortArgs = append(query.sortArgs, args...)
	return " ORDER BY (" + sql + ") " + direction
}

func userActivityTable(ctx context.Context, table string, alias string, columns ...interface{}) *goqu.SelectDataset {
	return dialect.From(table).Select(columns...).Where(goqu.C(userNameColumn).Eq(activityUser(ctx))).As(alias)
}

func sceneODatesTable(ctx context.Context) *goqu.SelectDataset {
	return userActivityTable(ctx, userSceneODatesTable, scenesODatesTable, sceneIDColumn, sceneODateColumn)
}

func imageOCountersTable(ctx context.Context, alias string) *goqu.SelectDataset {
	return userActivityTable(ctx, userImageOCountersTable, alias, goqu.C(imageIDColumn).As(idColumn), "o_counter")
}

func (t *viewHistoryTable) byUser(ctx context.Context) exp.Expression {
	return t.table.table.Col(userNameColumn).Eq(activityUser(ctx))
}

func sceneActivityCriterionHandler(c *models.IntCriterionInput, column string) criterionHandlerFunc {
	return func(ctx context.Context, f *filterBuilder) {
		if c != nil {
			activity, args := sceneActivitySQL(ctx, column, sceneTable+".id")
			clause, criterionArgs := getIntCriterionWhereClause("cast("+activity+" as int)", *c)
			f.addWhere(clause, append(args, criterionArgs...)...)
		}
	}
}

func imageOCounterCriterionHandler(c *models.IntCriterionInput) criterionHandlerFunc {
	return func(ctx context.Context, f *filterBuilder) {
		if c != nil {
			oCounter, args := imageOCounterSQL(ctx, imageTable+".id")
			clause, criterionArgs := getIntCriterionWhereClause(oCounter, *c)
			f.addWhere(clause, append(args, criterionArgs...)...)
		}
	}
}

func activityCountCriterionHandler(c *models.IntCriterionInput, activitySQL func(ctx context.Context) (string, []interface{})) criterionHandlerFunc {
	return func(ctx context.Context, f *filterBuilder) {
		if c != nil {
			table, args := activitySQL(ctx)
			clause, criterionArgs := getCountCriterionClause(sceneTable, table, sceneIDColumn, "", *c)
			f.addWhere(clause, append(args, criterionArgs...)...)
		}
	}
}

func loadSceneActivity(ctx context.Context, scenes []*models.Scene) error {
	byID := make(map[int]*models.Scene, len(scenes))
	ids := make([]int, 0, len(scenes))
	for _, s := range scenes {
		s.ResumeTime = 0
		s.PlayDuration = 0
		byID[s.ID] = s
		ids = append(ids, s.ID)
	}

	user := activityUser(ctx)
	table := goqu.T(userSceneActivityTable)

	return batchExec(ids, defaultBatchSize, func(batch []int) error {
		q := dialect.From(table).Select(
			table.Col(sceneIDColumn), table.Col("resume_time"), table.Col("play_duration"),
		).Where(table.Col(sceneIDColumn).In(batch), table.Col(userNameColumn).Eq(user))

		const single = false
		return queryFunc(ctx, q, single, func(rows *sqlx.Rows) error {
			var id int
			var resumeTime, playDuration float64
			if err := rows.Scan(&id, &resumeTime, &playDuration); err != nil {
				return err
			}

			if s := byID[id]; s != nil {
				s.ResumeTime = resumeTime
				s.PlayDuration = playDuration
			}
			return nil
		})
	})
}

// optionalActivity returns nil if the value is not set, and zero if it is null.
func optionalActivity(o models.OptionalFloat64) *float64 {
	if !o.Set {
		return nil
	}

	v := o.Value
	if o.Null {
		v = 0
	}
	return &v
}

// setSceneActivity sets the resume time and play duration of the scene. Nil
// values are not changed.
func setSceneActivity(ctx context.Context, id int, resumeTime *float64, playDuration *float64) error {
	if resumeTime == nil && playDuration == nil {
		return nil
	}

	const stmt = `INSERT INTO user_scene_activity (scene_id, user_name, resume_time, play_duration)
		VALUES (?, ?, COALESCE(?, 0), COALESCE(?, 0))
		ON CONFLICT (scene_id, user_name) DO UPDATE SET
			resume_time = COALESCE(?, resume_time),
			play_duration = COALESCE(?, play_duration)`

	if _, err := dbWrapper.Exec(ctx, stmt, id, activityUser(ctx), resumeTime, playDuration, resumeTime, playDuration); err != nil {
		return fmt.Errorf("setting the activity of scene %d: %w", id, err)
	}

	return nil
}

// saveSceneActivity sets the resume time of the scene and adds to its play
// duration. Nil values are not changed.
func saveSceneActivity(ctx context.Context, id int, resumeTime *float64, playDuration *float64) error {
	if resumeTime == nil && playDuration == nil {
		return nil
	}

	const stmt = `INSERT INTO user_scene_activity (scene_id, user_name, resume_time, play_duration)
		VALUES (?, ?, COALESCE(?, 0), COALESCE(?, 0))
		ON CONFLICT (scene_id, user_name) DO UPDATE SET
			resume_time = COALESCE(?, resume_time),
			play_duration = play_duration + COALESCE(?, 0)`

	if _, err := dbWrapper.Exec(ctx, stmt, id, activityUser(ctx), resumeTime, playDuration, resumeTime, playDuration); err != nil {
		return fmt.Errorf("saving the activity of scene %d: %w", id, err)
	}

	return nil
}

func resetSceneActivity(ctx context.Context, id int, resetResume bool, resetDuration bool) error {
	const stmt = `UPDATE user_scene_activity SET
		resume_time = CASE WHEN ? THEN 0 ELSE resume_time END,
		play_duration = CASE WHEN ? THEN 0 ELSE play_duration END
		WHERE scene_id = ? AND user_name = ?`

	if _, err := dbWrapper.Exec(ctx, stmt, resetResume, resetDuration, id, activityUser(ctx)); err != nil {
		return fmt.Errorf("resetting the activity of scene %d: %w", id, err)
	}

	return nil
}

func loadImageOCounters(ctx context.Context, images []*models.Image) error {
	byID := make(map[int]*models.Image, len(images))
	ids := make([]int, 0, len(images))
	for _, i := range images {
		i.OCounter = 0
		byID[i.ID] = i
		ids = append(ids, i.ID)
	}

	user := activityUser(ctx)
	table := goqu.T(userImageOCountersTable)

	return batchExec(ids, defaultBatchSize, func(batch []int) error {
		q := dialect.From(table).Select(
			table.Col(imageIDColumn), table.Col("o_counter"),
		).Where(table.Col(imageIDColumn).In(batch), table.Col(userNameColumn).Eq(user))

		const single = false
		return queryFunc(ctx, q, single, func(rows *sqlx.Rows) error {
			var id, oCounter int
			if err := rows.Scan(&id, &oCounter); err != nil {
				return err
			}

			if i := byID[id]; i != nil {
				i.OCounter = oCounter
			}
			return nil
		})
	})
}

// setImageOCounter deletes the row of a zero O counter.
func setImageOCounter(ctx context.Context, id int, oCounter int) error {
	stmt := `INSERT INTO user_image_o_counters (image_id, user_name, o_counter) VALUES (?, ?, ?)
		ON CONFLICT (image_id, user_name) DO UPDATE SET o_counter = excluded.o_counter`
	args := []interface{}{id, activityUser(ctx), oCounter}
	if oCounter == 0 {
		stmt = "DELETE FROM user_image_o_counters WHERE image_id = ? AND user_name = ?"
		args = args[:2]
	}

	if _, err := dbWrapper.Exec(ctx, stmt, args...); err != nil {
		return fmt.Errorf("setting the o-counter of image %d: %w", id, err)
	}

	return nil
}

// addImageOCounter adds n to the O counter of the image, down to zero.
func addImageOCounter(ctx context.Context, id int, n int) error {
	const stmt = `INSERT INTO user_image_o_counters (image_id, user_name, o_counter) VALUES (?, ?, MAX(?, 0))
		ON CONFLICT (image_id, user_name) DO UPDATE SET o_counter = MAX(o_counter + ?, 0)`

	if _, err := dbWrapper.Exec(ctx, stmt, id, activityUser(ctx), n, n); err != nil {
		return fmt.Errorf("changing the o-counter of image %d: %w", id, err)
	}

	return nil
}
