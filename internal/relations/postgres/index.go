// Package postgres implements the rebuildable owner projection with pgx.
package postgres

import (
	"cmp"
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kongken/urlo/internal/relations"
	"github.com/kongken/urlo/internal/url"
)

type Index struct {
	pool    *pgxpool.Pool
	space   string
	timeout time.Duration
}

var _ relations.Index = (*Index)(nil)

func New(pool *pgxpool.Pool, space string, timeout time.Duration) (*Index, error) {
	if pool == nil || space == "" || timeout < 0 {
		return nil, errors.New("postgres owner index: pool, space and valid timeout required")
	}
	return &Index{pool: pool, space: space, timeout: cmp.Or(timeout, time.Second)}, nil
}

// Check verifies the expected schema without applying migrations.
func (i *Index) Check(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, i.timeout)
	defer cancel()
	rows, err := i.pool.Query(ctx, "SELECT link_space, code, owner_id, link_created_at, indexed_at FROM owner_link_index LIMIT 0")
	if err != nil {
		return err
	}
	rows.Close()
	return rows.Err()
}

// Refresh takes a per-key transaction lock before reading S3, serializing late
// notifications and reconciliation across replicas. This is not a S3/SQL
// transaction: a concurrent S3 mutation can still require a later refresh.
func (i *Index) Refresh(parent context.Context, code string, source relations.RecordGetter) error {
	if !url.IsValidCode(code) || source == nil {
		return errors.New("postgres owner index: invalid code or source")
	}
	ctx, cancel := context.WithTimeout(parent, i.timeout)
	defer cancel()
	tx, err := i.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		_ = tx.Rollback(cleanupCtx)
	}()
	// Hash collisions only serialize unrelated refreshes. A slash delimiter is
	// unambiguous because valid codes cannot contain slashes.
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", i.space+"/"+code); err != nil {
		return err
	}
	r, err := source.Get(ctx, code)
	if err != nil && !errors.Is(err, url.ErrNotFound) {
		return err // never interpret a timeout/auth/backend error as deletion
	}
	if err == nil && (r == nil || r.Code != code || r.OwnerID != "" && r.CreatedAt.IsZero()) {
		return errors.New("postgres owner index: invalid primary record")
	}
	if errors.Is(err, url.ErrNotFound) || r.OwnerID == "" {
		_, err = tx.Exec(ctx, "DELETE FROM owner_link_index WHERE link_space = $1 AND code = $2", i.space, code)
	} else {
		_, err = tx.Exec(ctx, `
INSERT INTO owner_link_index (link_space, code, owner_id, link_created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (link_space, code) DO UPDATE
SET owner_id = EXCLUDED.owner_id,
    link_created_at = EXCLUDED.link_created_at,
    indexed_at = now()
WHERE owner_link_index.owner_id IS DISTINCT FROM EXCLUDED.owner_id
   OR owner_link_index.link_created_at IS DISTINCT FROM EXCLUDED.link_created_at`, i.space, code, r.OwnerID, r.CreatedAt)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (i *Index) List(parent context.Context, owner string, after *url.OwnerCursor, limit int) ([]relations.Ref, error) {
	if limit < 1 || limit > 1000 {
		return nil, url.ErrInvalidPage
	}
	if owner == "" {
		return []relations.Ref{}, nil
	}
	ctx, cancel := context.WithTimeout(parent, i.timeout)
	defer cancel()
	query := `SELECT code, link_created_at FROM owner_link_index
WHERE link_space = $1 AND owner_id = $2`
	args := []any{i.space, owner}
	if after != nil {
		query += " AND (link_created_at, code) < ($3::timestamptz, $4::text) ORDER BY link_created_at DESC, code DESC LIMIT $5"
		args = append(args, after.CreatedAt, after.Code, limit)
	} else {
		query += " ORDER BY link_created_at DESC, code DESC LIMIT $3"
		args = append(args, limit)
	}
	rows, err := i.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	refs := make([]relations.Ref, 0, limit)
	for rows.Next() {
		var ref relations.Ref
		if err := rows.Scan(&ref.Code, &ref.CreatedAt); err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	return refs, rows.Err()
}

func (i *Index) ScanCodes(parent context.Context, after string, limit int) ([]string, error) {
	if limit < 1 || limit > 1000 {
		return nil, url.ErrInvalidPage
	}
	ctx, cancel := context.WithTimeout(parent, i.timeout)
	defer cancel()
	rows, err := i.pool.Query(ctx, `SELECT code FROM owner_link_index
WHERE link_space = $1 AND code > $2 ORDER BY code LIMIT $3`, i.space, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	codes := make([]string, 0, limit)
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, rows.Err()
}
