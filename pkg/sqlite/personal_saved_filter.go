package sqlite

import (
	"context"
	"fmt"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	"github.com/jmoiron/sqlx"

	"github.com/stashapp/stash/pkg/models"
	"github.com/stashapp/stash/pkg/txn"
)

// userSavedFiltersTable has the personal saved filters of the users. It is
// created when the database is opened rather than by a schema migration, so
// that the database stays compatible with the upstream migrations. Its filter
// columns are those of saved_filters: a migration that changes saved_filters
// must be repeated for it (see TestSavedFilterMigrationsMirrored).
const userSavedFiltersTable = "user_saved_filters"

var userSavedFiltersSchema = []string{
	`CREATE TABLE IF NOT EXISTS user_saved_filters (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		user_name TEXT NOT NULL,
		mode VARCHAR(255) NOT NULL,
		name VARCHAR(510) NOT NULL,
		find_filter BLOB,
		object_filter BLOB,
		ui_options BLOB
	)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS index_user_saved_filters_on_user_name_mode_name ON user_saved_filters (user_name, mode, name)`,
	// number the personal filters above every shared filter
	fmt.Sprintf(`INSERT INTO sqlite_sequence (name, seq) SELECT 'user_saved_filters', %d
		WHERE NOT EXISTS (SELECT 1 FROM sqlite_sequence WHERE name = 'user_saved_filters')`, models.PersonalSavedFilterIDOffset),
}

func ensureUserSavedFiltersTable(db *sqlx.DB) error {
	for _, stmt := range userSavedFiltersSchema {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("creating %s table: %w", userSavedFiltersTable, err)
		}
	}

	return nil
}

var userSavedFiltersTableMgr = &table{
	table:    goqu.T(userSavedFiltersTable),
	idColumn: goqu.T(userSavedFiltersTable).Col(idColumn),
}

type personalSavedFilterRow struct {
	savedFilterRow
	UserName string `db:"user_name"`
}

// PersonalSavedFilterStore reads and writes the personal saved filters of the
// users.
type PersonalSavedFilterStore struct {
	tableMgr *table
}

func NewPersonalSavedFilterStore() *PersonalSavedFilterStore {
	return &PersonalSavedFilterStore{
		tableMgr: userSavedFiltersTableMgr,
	}
}

func (qb *PersonalSavedFilterStore) table() exp.IdentifierExpression {
	return qb.tableMgr.table
}

// ofUser returns the condition that a filter belongs to the user.
func (qb *PersonalSavedFilterStore) ofUser(user string) exp.Expression {
	return qb.table().Col(userNameColumn).Eq(user)
}

func (qb *PersonalSavedFilterStore) selectDataset(user string) *goqu.SelectDataset {
	return dialect.From(qb.table()).Select(qb.table().All()).Where(qb.ofUser(user))
}

func (qb *PersonalSavedFilterStore) Create(ctx context.Context, user string, newObject *models.SavedFilter) error {
	r := personalSavedFilterRow{UserName: user}
	r.fromSavedFilter(*newObject)

	id, err := qb.tableMgr.insertID(ctx, r)
	if err != nil {
		return err
	}

	created, err := qb.Find(ctx, user, id)
	if err != nil {
		return fmt.Errorf("finding after create: %w", err)
	}

	*newObject = *created
	return nil
}

func (qb *PersonalSavedFilterStore) Update(ctx context.Context, user string, updatedObject *models.SavedFilter) error {
	r := personalSavedFilterRow{UserName: user}
	r.fromSavedFilter(*updatedObject)

	q := dialect.Update(qb.table()).Prepared(true).Set(r).Where(qb.tableMgr.byID(updatedObject.ID), qb.ofUser(user))
	return qb.execOne(ctx, q, updatedObject.ID)
}

func (qb *PersonalSavedFilterStore) Destroy(ctx context.Context, user string, id int) error {
	q := dialect.Delete(qb.table()).Where(qb.tableMgr.byID(id), qb.ofUser(user))
	return qb.execOne(ctx, q, id)
}

// execOne executes a statement on the filter with the id, and returns a
// NotFoundError if the user has no such filter.
func (qb *PersonalSavedFilterStore) execOne(ctx context.Context, q sqler, id int) error {
	result, err := exec(ctx, q)
	if err != nil {
		return fmt.Errorf("writing %s: %w", userSavedFiltersTable, err)
	}

	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return &NotFoundError{ID: id, Table: userSavedFiltersTable}
	}

	return nil
}

// Find returns the filter of the user with the id, or nil if the user has no
// such filter.
func (qb *PersonalSavedFilterStore) Find(ctx context.Context, user string, id int) (*models.SavedFilter, error) {
	ret, err := qb.getMany(ctx, qb.selectDataset(user).Where(qb.tableMgr.byID(id)))
	if err != nil || len(ret) == 0 {
		return nil, err
	}

	return ret[0], nil
}

func (qb *PersonalSavedFilterStore) FindByMode(ctx context.Context, user string, mode models.FilterMode) ([]*models.SavedFilter, error) {
	t := qb.table()

	// groups include the movies filters, as in SavedFilterStore.FindByMode
	var whereClause exp.Expression
	if mode == models.FilterModeGroups || mode == models.FilterModeMovies {
		whereClause = goqu.Or(
			t.Col("mode").Eq(models.FilterModeGroups),
			t.Col("mode").Eq(models.FilterModeMovies),
		)
	} else {
		whereClause = t.Col("mode").Eq(mode)
	}

	return qb.getMany(ctx, qb.selectDataset(user).Prepared(true).Where(whereClause).Order(t.Col("name").Asc()))
}

func (qb *PersonalSavedFilterStore) All(ctx context.Context, user string) ([]*models.SavedFilter, error) {
	return qb.getMany(ctx, qb.selectDataset(user).Order(qb.table().Col("name").Asc()))
}

func (qb *PersonalSavedFilterStore) getMany(ctx context.Context, q *goqu.SelectDataset) ([]*models.SavedFilter, error) {
	const single = false
	var ret []*models.SavedFilter
	if err := queryFunc(ctx, q, single, func(r *sqlx.Rows) error {
		var f personalSavedFilterRow
		if err := r.StructScan(&f); err != nil {
			return err
		}

		s := f.resolve()
		s.Personal = true
		ret = append(ret, s)
		return nil
	}); err != nil {
		return nil, err
	}

	return ret, nil
}

// MoveUserSavedFilters gives the personal saved filters of the user from to
// the user to, or deletes them if to is nil. The filters of from replace those
// of to with the same mode and name.
func (db *Database) MoveUserSavedFilters(ctx context.Context, from string, to *string) error {
	return txn.WithTxn(ctx, db, func(ctx context.Context) error {
		if to == nil {
			if _, err := dbWrapper.Exec(ctx, "DELETE FROM user_saved_filters WHERE user_name = ?", from); err != nil {
				return fmt.Errorf("deleting the saved filters of %q: %w", from, err)
			}
			return nil
		}

		if from == *to {
			return nil
		}

		stmts := []string{
			`DELETE FROM user_saved_filters WHERE user_name = :to AND EXISTS (
				SELECT 1 FROM user_saved_filters AS f
				WHERE f.user_name = :from AND f.mode = user_saved_filters.mode AND f.name = user_saved_filters.name
			)`,
			`UPDATE user_saved_filters SET user_name = :to WHERE user_name = :from`,
		}
		arg := map[string]interface{}{"from": from, "to": *to}
		for _, stmt := range stmts {
			if _, err := dbWrapper.NamedExec(ctx, stmt, arg); err != nil {
				return fmt.Errorf("moving the saved filters of %q to %q: %w", from, *to, err)
			}
		}

		return nil
	})
}
