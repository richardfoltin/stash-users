package sqlite

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/sliceutil"
)

// sceneVisibilityTable has the minimum role that can see the restricted scenes.
const sceneVisibilityTable = "scene_visibility"

var sceneVisibilitySchema = []string{
	`CREATE TABLE IF NOT EXISTS scene_visibility (
		scene_id INTEGER PRIMARY KEY REFERENCES scenes(id) ON DELETE CASCADE,
		min_role TEXT NOT NULL CHECK (min_role IN ('MODIFY', 'ADMIN'))
	)`,
	`CREATE INDEX IF NOT EXISTS index_scene_visibility_on_min_role ON scene_visibility (min_role)`,
	// rows of scenes that were deleted while foreign keys were disabled
	`DELETE FROM scene_visibility WHERE scene_id NOT IN (SELECT id FROM scenes)`,
}

func ensureSceneVisibilityTable(db *sqlx.DB) error {
	for _, stmt := range sceneVisibilitySchema {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("creating %s table: %w", sceneVisibilityTable, err)
		}
	}

	return nil
}

// hiddenVisibilities returns the scene visibilities that are hidden in the
// context, quoted for SQL. Returns nil if all scenes are visible.
func hiddenVisibilities(ctx context.Context) []string {
	role, restricted := models.ViewerRole(ctx)
	if !restricted {
		return nil
	}

	var ret []string
	for _, v := range []models.RoleEnum{models.RoleEnumModify, models.RoleEnumAdmin} {
		if !role.Implies(v) {
			ret = append(ret, "'"+v.String()+"'")
		}
	}

	return ret
}

// sceneVisibleClause returns an SQL condition that is true if the scene with
// the ID in the column is visible, or the ID is NULL. Returns an empty string
// if all scenes are visible.
func sceneVisibleClause(ctx context.Context, sceneIDColumn string) string {
	hidden := hiddenVisibilities(ctx)
	if len(hidden) == 0 {
		return ""
	}

	return fmt.Sprintf("NOT EXISTS (SELECT 1 FROM %s AS hidden_scene WHERE hidden_scene.scene_id = %s AND hidden_scene.min_role IN (%s))",
		sceneVisibilityTable, sceneIDColumn, strings.Join(hidden, ", "))
}

func andSceneVisible(ctx context.Context, sceneIDColumn string) string {
	if c := sceneVisibleClause(ctx, sceneIDColumn); c != "" {
		return " AND " + c
	}

	return ""
}

func whereSceneVisibleSQL(ctx context.Context, sceneIDColumn string) string {
	if c := sceneVisibleClause(ctx, sceneIDColumn); c != "" {
		return " WHERE " + c
	}

	return ""
}

func withSceneVisible(ctx context.Context, template string, sceneIDColumn string) string {
	return fmt.Sprintf(template, andSceneVisible(ctx, sceneIDColumn))
}

func whereSceneVisible(ctx context.Context, q *goqu.SelectDataset, sceneIDColumn string) *goqu.SelectDataset {
	if c := sceneVisibleClause(ctx, sceneIDColumn); c != "" {
		return q.Where(goqu.L(c))
	}

	return q
}

// fileVisibleClause returns an SQL condition that is true if the file does not
// belong to a scene, or belongs to a visible scene.
func fileVisibleClause(ctx context.Context, fileIDColumn string) string {
	visible := sceneVisibleClause(ctx, "file_scene.scene_id")
	if visible == "" {
		return ""
	}

	return fmt.Sprintf("(NOT EXISTS (SELECT 1 FROM %[1]s AS file_scene WHERE file_scene.file_id = %[2]s) OR EXISTS (SELECT 1 FROM %[1]s AS file_scene WHERE file_scene.file_id = %[2]s AND %[3]s))",
		scenesFilesTable, fileIDColumn, visible)
}

// visibleClause returns the visibility condition of the rows of the table, or
// an empty string if its rows cannot be hidden.
func visibleClause(ctx context.Context, table string) string {
	switch table {
	case sceneTable:
		return sceneVisibleClause(ctx, sceneTable+".id")
	case sceneMarkerTable:
		return sceneVisibleClause(ctx, sceneMarkerTable+".scene_id")
	case fileTable:
		return fileVisibleClause(ctx, fileTable+".id")
	}

	return ""
}

func hiddenSceneIDs(ctx context.Context, ids []int) ([]int, error) {
	hidden := hiddenVisibilities(ctx)
	if len(hidden) == 0 || len(ids) == 0 {
		return nil, nil
	}

	var ret []int
	if err := batchExec(ids, defaultBatchSize, func(batch []int) error {
		query := fmt.Sprintf("SELECT scene_id FROM %s WHERE scene_id IN %s AND min_role IN (%s)",
			sceneVisibilityTable, getInBinding(len(batch)), strings.Join(hidden, ", "))

		args := make([]interface{}, len(batch))
		for i, id := range batch {
			args[i] = id
		}

		var found []int
		if err := dbWrapper.Select(ctx, &found, query, args...); err != nil {
			return err
		}

		ret = append(ret, found...)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("getting hidden scenes: %w", err)
	}

	return ret, nil
}

// checkScenesVisible returns a NotFoundError if any of the scenes is hidden.
func checkScenesVisible(ctx context.Context, ids ...int) error {
	hidden, err := hiddenSceneIDs(ctx, ids)
	if err != nil {
		return err
	}

	if len(hidden) > 0 {
		return &NotFoundError{ID: hidden[0], Table: sceneTable}
	}

	return nil
}

func visibleSceneIDs(ctx context.Context, ids []int) ([]int, error) {
	hidden, err := hiddenSceneIDs(ctx, ids)
	if err != nil || len(hidden) == 0 {
		return ids, err
	}

	return sliceutil.Exclude(ids, hidden), nil
}

// updateSceneIDs checks that the scenes of the update are visible, and keeps
// the joins to hidden scenes when the scenes are set.
func updateSceneIDs(ctx context.Context, update models.UpdateIDs, joins *joinTable, id int) (models.UpdateIDs, error) {
	if err := checkScenesVisible(ctx, update.IDs...); err != nil {
		return update, err
	}

	if update.Mode != models.RelationshipUpdateModeSet || len(hiddenVisibilities(ctx)) == 0 {
		return update, nil
	}

	existing, err := joins.get(ctx, id)
	if err != nil {
		return update, err
	}

	hidden, err := hiddenSceneIDs(ctx, existing)
	if err != nil {
		return update, err
	}

	update.IDs = append(slices.Clone(update.IDs), hidden...)
	return update, nil
}

func sceneVisibilityCriterionHandler(c *models.RoleCriterionInput) criterionHandlerFunc {
	return func(ctx context.Context, f *filterBuilder) {
		if c == nil {
			return
		}

		column := fmt.Sprintf("COALESCE((SELECT sv.min_role FROM %s AS sv WHERE sv.scene_id = scenes.id), '%s')", sceneVisibilityTable, models.RoleEnumRead)
		values := make([]string, len(c.Value))
		for i, v := range c.Value {
			values[i] = v.String()
		}

		enumCriterionHandler(c.Modifier, values, column)(ctx, f)
	}
}

// SetDefaultSceneVisibility sets the function that returns the visibility of new scenes.
func (db *Database) SetDefaultSceneVisibility(fn func() models.RoleEnum) {
	db.Scene.defaultVisibility = fn
}

func (qb *SceneStore) newSceneVisibility() models.RoleEnum {
	if qb.defaultVisibility == nil {
		return models.RoleEnumRead
	}

	if ret := qb.defaultVisibility(); ret.IsValid() {
		return ret
	}

	return models.RoleEnumRead
}

func (qb *SceneStore) GetVisibility(ctx context.Context, id int) (models.RoleEnum, error) {
	query := fmt.Sprintf("SELECT min_role FROM %s WHERE scene_id = ?", sceneVisibilityTable)

	var ret []string
	if err := dbWrapper.Select(ctx, &ret, query, id); err != nil {
		return "", fmt.Errorf("getting visibility of scene %d: %w", id, err)
	}

	if len(ret) == 0 {
		return models.RoleEnumRead, nil
	}

	return models.RoleEnum(ret[0]), nil
}

func (qb *SceneStore) GetManyVisibility(ctx context.Context, ids []int) ([]models.RoleEnum, error) {
	ret := make([]models.RoleEnum, len(ids))
	for i := range ret {
		ret[i] = models.RoleEnumRead
	}

	idToIndex := idToIndexMap(ids)
	table := goqu.T(sceneVisibilityTable)
	if err := batchExec(ids, defaultBatchSize, func(batch []int) error {
		q := dialect.From(table).Select(table.Col(sceneIDColumn), table.Col("min_role")).Where(table.Col(sceneIDColumn).In(batch))

		const single = false
		return queryFunc(ctx, q, single, func(rows *sqlx.Rows) error {
			var id int
			var role string
			if err := rows.Scan(&id, &role); err != nil {
				return err
			}

			ret[idToIndex[id]] = models.RoleEnum(role)
			return nil
		})
	}); err != nil {
		return nil, fmt.Errorf("getting visibility of scenes: %w", err)
	}

	return ret, nil
}

func (qb *SceneStore) setVisibility(ctx context.Context, id int, role models.RoleEnum) error {
	if !role.IsValid() {
		return fmt.Errorf("invalid scene visibility: %q", role)
	}

	var err error
	if role == models.RoleEnumRead {
		_, err = dbWrapper.Exec(ctx, "DELETE FROM "+sceneVisibilityTable+" WHERE scene_id = ?", id)
	} else {
		_, err = dbWrapper.Exec(ctx, "INSERT INTO "+sceneVisibilityTable+" (scene_id, min_role) VALUES (?, ?) ON CONFLICT (scene_id) DO UPDATE SET min_role = excluded.min_role", id, role.String())
	}

	if err != nil {
		return fmt.Errorf("setting visibility of scene %d: %w", id, err)
	}

	return nil
}
