package historycatalog

import (
	"context"
	"path/filepath"
	"strings"

	"reasonix/internal/state/sessionstore"
	"reasonix/internal/state/store"
)

// sourceProjectionUnchanged reports whether the stored projection for one
// source can be trusted without reading the transcript again. A non-ok health
// means the last read failed (corrupt) or never completed (stale): the stored
// fingerprints then describe bytes the projection could not use, so equal
// size/mtime cannot license a skip.
func sourceProjectionUnchanged(queryErr error, oldHealth, oldContent, content, oldMeta, meta, oldDigest, digest string) bool {
	return queryErr == nil && oldHealth == "ok" && oldContent == content && oldMeta == meta && (digest == "" || digest == oldDigest)
}

// rootProjectionHealthy reports whether every source currently projected for
// root is usable. It mirrors the visibility rule Search applies (health='ok'
// and not awaiting removal): a row the reader cannot trust is a retry
// candidate, not a reason to skip the directory.
func (c *Catalog) rootProjectionHealthy(ctx context.Context, rootPath string) bool {
	var unhealthy int
	if err := c.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM history_sources WHERE root=? AND (health<>'ok' OR missing_since<>0)`,
		rootPath).Scan(&unhealthy); err != nil {
		return false
	}
	return unhealthy == 0
}

func (c *Catalog) tryAppendPath(ctx context.Context, root Root, path string, generation int64, appendFrom,
	oldMessageCount int, oldRevision, revision int64, digest, contentFingerprint, metaFingerprint string) (bool, error) {
	if appendFrom != oldMessageCount || revision != oldRevision+1 {
		return false, nil
	}
	index, err := sessionstore.LoadSessionDisplayIndex(store.SessionDisplayIndex(path))
	if err != nil || !index.RevisionKnown || index.Revision != revision || index.ContentDigest != digest || index.MessageCount < appendFrom {
		return false, nil
	}
	tail, checkedIndex, err := sessionstore.LoadSessionDisplayMessageRange(path, appendFrom, index.MessageCount)
	if err != nil || checkedIndex.ContentDigest != digest || checkedIndex.Revision != revision {
		return false, nil
	}
	meta, _, _ := sessionstore.LoadBranchMeta(path)
	lastActivity := max(int64(0), sessionstore.SessionContentModTime(path).UnixMilli())
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE history_sources SET root=?,source=?,scope=?,workspace_root=?,content_revision=?,
		content_digest=?,content_fingerprint=?,meta_fingerprint=?,message_count=?,indexed_message_count=?,custom_title=?,topic_id=?,
		topic_title=?,preview=?,created_at=?,last_activity_at=?,health='ok',missing_since=0,
		seen_generation=CASE WHEN ?>0 THEN ? ELSE seen_generation END,last_error=''
		WHERE path=? AND content_revision=? AND indexed_message_count=?`, root.Path, root.Source, root.Scope, root.WorkspaceRoot,
		revision, digest, contentFingerprint, metaFingerprint, index.MessageCount, index.MessageCount, meta.CustomTitle, meta.TopicID,
		meta.TopicTitle, meta.Preview, meta.CreatedAt.UnixMilli(), lastActivity, generation, generation, path, oldRevision, oldMessageCount)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	updated, _ := result.RowsAffected()
	if updated != 1 {
		_ = tx.Rollback()
		return false, nil
	}
	for _, doc := range documents(tail) {
		result, err := tx.ExecContext(ctx, `INSERT INTO history_documents(source_path,message_index,part_index,role,kind,tool_name,token_count)
			VALUES(?,?,?,?,?,?,?)`, path, appendFrom+doc.message, doc.part, doc.role, doc.kind, doc.tool, doc.count)
		if err != nil {
			_ = tx.Rollback()
			return false, err
		}
		rowID, err := result.LastInsertId()
		if err != nil {
			_ = tx.Rollback()
			return false, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO history_fts(rowid,terms) VALUES(?,?)`, rowID, doc.terms); err != nil {
			_ = tx.Rollback()
			return false, err
		}
	}
	newRevision, err := bump(ctx, tx)
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	c.publish(newRevision, []string{root.Path}, "source-appended")
	return true, nil
}

// RecordReadFailure marks a source the reader could not open as corrupt and
// queues it for reconciliation: an unreadable transcript cannot serve a hit,
// so leaving health ok would let Indexed == Total certify an empty result.
// The scan already retries every non-ok source, so retries stay bounded
// there; a row that is already corrupt is not re-queued by each search.
func (c *Catalog) RecordReadFailure(ctx context.Context, path string, cause error) bool {
	if c == nil || strings.TrimSpace(path) == "" {
		return false
	}
	path = filepath.Clean(path)
	message := ""
	if cause != nil {
		message = cause.Error()
	}
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return false
	}
	result, err := tx.ExecContext(ctx, `UPDATE history_sources SET health='corrupt',last_error=? WHERE path=? AND health='ok'`,
		message, path)
	if err != nil {
		_ = tx.Rollback()
		return false
	}
	updated, _ := result.RowsAffected()
	if updated != 1 {
		_ = tx.Rollback()
		return false
	}
	revision, err := bump(ctx, tx)
	if err != nil {
		_ = tx.Rollback()
		return false
	}
	if err := tx.Commit(); err != nil {
		return false
	}
	c.publish(revision, nil, "source-unreadable")
	return c.EnqueueExisting(ctx, path)
}
