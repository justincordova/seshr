package opencode

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"github.com/justincordova/seshr/internal/backend"
)

// Scan returns metadata for every non-archived session in the OC database.
// Populates token + cost aggregates in a second query grouped by session_id.
//
// Two-query approach keeps the LEFT JOIN simple (one row per session) and
// avoids a multi-table join-with-aggregate on what is a 40k-row table.
//
// Performance: on the author's 1359-session DB (messages ≈ 43k, parts ≈ 167k)
// this takes ~600ms — slightly above the 500ms design target. The aggregate
// query dominates (SUM with five json_extract calls per row). If this becomes
// a user-visible regression, the mitigation is a ~/.seshr/opencode_meta.db
// cache keyed by (session_id, time_updated); for now we accept it.
//
// TODO(post-v1): Scan cache for OpenCode when user-session count exceeds ~2k.
func (s *Store) Scan(ctx context.Context) ([]backend.SessionMeta, error) {
	sessRows, err := s.querySessions(ctx)
	if err != nil {
		return nil, err
	}
	if len(sessRows) == 0 {
		return nil, nil
	}

	agg, err := s.queryAggregates(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]backend.SessionMeta, 0, len(sessRows))
	for _, r := range sessRows {
		a := agg[r.id]
		out = append(out, backend.SessionMeta{
			ID:         r.id,
			Kind:       s.Kind(),
			Project:    r.projectName(),
			Directory:  r.directory,
			Title:      r.title,
			TokenCount: a.tokens,
			CostUSD:    a.cost,
			CreatedAt:  time.UnixMilli(r.timeCreated),
			UpdatedAt:  time.UnixMilli(r.timeUpdated),
			SizeBytes:  0, // OC size is not meaningful (no file per session).
			HasBackup:  s.hasBackup(r.id),
		})
	}
	return out, nil
}

// sessionScanRow holds the columns we read from the session table.
type sessionScanRow struct {
	id           string
	directory    string
	title        string
	timeCreated  int64
	timeUpdated  int64
	projName     sql.NullString
	projWorktree sql.NullString
}

// projectName falls back to filepath.Base(directory) when project.name is
// null or empty. Matches design §4.2.
func (r sessionScanRow) projectName() string {
	if r.projName.Valid && r.projName.String != "" {
		return r.projName.String
	}
	if r.directory != "" {
		return filepath.Base(r.directory)
	}
	if r.projWorktree.Valid {
		return filepath.Base(r.projWorktree.String)
	}
	return "opencode"
}

// querySessions reads every non-archived session with its project join.
func (s *Store) querySessions(ctx context.Context) ([]sessionScanRow, error) {
	const q = `
		SELECT s.id, s.directory, s.title, s.time_created, s.time_updated,
		       p.name, p.worktree
		FROM session s
		LEFT JOIN project p ON s.project_id = p.id
		WHERE s.time_archived IS NULL
		ORDER BY s.time_updated DESC
	`
	rows, err := s.conns.read.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query sessions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []sessionScanRow
	for rows.Next() {
		var r sessionScanRow
		if err := rows.Scan(
			&r.id, &r.directory, &r.title,
			&r.timeCreated, &r.timeUpdated,
			&r.projName, &r.projWorktree,
		); err != nil {
			return nil, fmt.Errorf("scan session row: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter sessions: %w", err)
	}
	return out, nil
}

// aggRow is the per-session sum of assistant-message tokens + costs.
type aggRow struct {
	tokens int
	cost   float64
}

// queryAggregates pulls assistant-only message token + cost sums grouped
// by session_id.
//
// Token components live at nested JSON paths; COALESCE guards against rows
// with partial data. json_extract returns NULL for missing paths, so
// COALESCE(..., 0) keeps the SUM well-defined.
func (s *Store) queryAggregates(ctx context.Context) (map[string]aggRow, error) {
	const q = `
		SELECT session_id,
		       COALESCE(SUM(
		           COALESCE(CAST(json_extract(data, '$.tokens.input') AS INTEGER), 0) +
		           COALESCE(CAST(json_extract(data, '$.tokens.output') AS INTEGER), 0) +
		           COALESCE(CAST(json_extract(data, '$.tokens.reasoning') AS INTEGER), 0) +
		           COALESCE(CAST(json_extract(data, '$.tokens.cache.read') AS INTEGER), 0) +
		           COALESCE(CAST(json_extract(data, '$.tokens.cache.write') AS INTEGER), 0)
		       ), 0) AS tokens,
		       COALESCE(SUM(CAST(json_extract(data, '$.cost') AS REAL)), 0) AS cost
		FROM message
		WHERE json_extract(data, '$.role') = 'assistant'
		GROUP BY session_id
	`
	rows, err := s.conns.read.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query aggregates: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := make(map[string]aggRow, 1024)
	for rows.Next() {
		var id string
		var a aggRow
		if err := rows.Scan(&id, &a.tokens, &a.cost); err != nil {
			return nil, fmt.Errorf("scan agg row: %w", err)
		}
		out[id] = a
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iter aggs: %w", err)
	}
	return out, nil
}
