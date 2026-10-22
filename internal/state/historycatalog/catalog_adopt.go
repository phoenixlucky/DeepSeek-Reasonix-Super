package historycatalog

import (
	"context"
	"database/sql"
	"errors"
	"os"

	"reasonix/internal/state/sessionstore"
)

// measureUnseen splits the sources this scan did not read into ones that still
// name a transcript and ones whose transcript is confirmed absent. It runs
// outside the write transaction: a stat on a slow filesystem must not hold the
// write lock. A source in neither list is never swept.
func (c *Catalog) measureUnseen(ctx context.Context, root Root, generation int64) (live, absent []string, err error) {
	rows, err := c.db.QueryContext(ctx, `SELECT path FROM history_sources WHERE root=? AND seen_generation<>?`, root.Path, generation)
	if err != nil {
		return nil, nil, err
	}
	var unseen []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			rows.Close()
			return nil, nil, err
		}
		unseen = append(unseen, path)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, err
	}
	rows.Close()
	for _, path := range unseen {
		if _, statErr := os.Stat(path); statErr != nil && errors.Is(statErr, os.ErrNotExist) {
			absent = append(absent, path)
			continue
		}
		// A path that cannot be stat'd is not evidence of absence.
		if !root.Archive && !sessionstore.IsVisibleSession(path) {
			continue
		}
		live = append(live, path)
	}
	return live, absent, nil
}

// adoptLiveSources marks the measured-live sources as seen by this scan, so
// neither the missing sweep nor the grace removal touches them.
func (c *Catalog) adoptLiveSources(ctx context.Context, tx *sql.Tx, generation int64, live []string) error {
	for _, path := range live {
		if _, err := tx.ExecContext(ctx, `UPDATE history_sources SET seen_generation=? WHERE path=?`, generation, path); err != nil {
			return err
		}
	}
	return nil
}

// sweepAbsent records only the sources whose transcript a stat confirmed gone,
// so a row another process indexed during this scan is never swept as missing.
func (c *Catalog) sweepAbsent(ctx context.Context, tx *sql.Tx, now int64, absent []string) error {
	for _, path := range absent {
		if _, err := tx.ExecContext(ctx, `UPDATE history_sources SET missing_since=CASE WHEN missing_since=0 THEN ? ELSE missing_since END,
        health='missing' WHERE path=?`, now, path); err != nil {
			return err
		}
	}
	return nil
}

// applyScanResult adopts the measured-live sources and sweeps only the ones a
// stat confirmed gone.
func (c *Catalog) applyScanResult(ctx context.Context, tx *sql.Tx, now, generation int64, live, absent []string) error {
	if err := c.adoptLiveSources(ctx, tx, generation, live); err != nil {
		return err
	}
	return c.sweepAbsent(ctx, tx, now, absent)
}
