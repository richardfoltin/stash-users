package sqlite

import (
	"context"

	"github.com/doug-martin/goqu/v9"
	"github.com/jmoiron/sqlx"
)

// oCounterManager manages the O counter of images. The O counter of the
// activity user is read and written.
type oCounterManager struct {
	tableMgr *table
}

func (qb *oCounterManager) getOCounter(ctx context.Context, id int) (int, error) {
	q := dialect.From(userImageOCountersTable).Select("o_counter").Where(goqu.Ex{imageIDColumn: id, userNameColumn: activityUser(ctx)})

	const single = true
	var ret int
	if err := queryFunc(ctx, q, single, func(rows *sqlx.Rows) error {
		if err := rows.Scan(&ret); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return 0, err
	}

	return ret, nil
}

func (qb *oCounterManager) IncrementOCounter(ctx context.Context, id int) (int, error) {
	if err := qb.tableMgr.checkIDExists(ctx, id); err != nil {
		return 0, err
	}

	if err := addImageOCounter(ctx, id, 1); err != nil {
		return 0, err
	}

	return qb.getOCounter(ctx, id)
}

func (qb *oCounterManager) DecrementOCounter(ctx context.Context, id int) (int, error) {
	if err := qb.tableMgr.checkIDExists(ctx, id); err != nil {
		return 0, err
	}

	if err := addImageOCounter(ctx, id, -1); err != nil {
		return 0, err
	}

	return qb.getOCounter(ctx, id)
}

func (qb *oCounterManager) ResetOCounter(ctx context.Context, id int) (int, error) {
	if err := qb.tableMgr.checkIDExists(ctx, id); err != nil {
		return 0, err
	}

	if err := setImageOCounter(ctx, id, 0); err != nil {
		return 0, err
	}

	return qb.getOCounter(ctx, id)
}
