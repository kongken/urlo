// Package postgres implements the rebuildable owner projection with Ent.
package postgres

import (
	"cmp"
	"context"
	stdsql "database/sql"
	"errors"
	"time"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"github.com/kongken/urlo/internal/relations"
	"github.com/kongken/urlo/internal/relations/ent"
	"github.com/kongken/urlo/internal/relations/ent/ownerlinkindex"
	"github.com/kongken/urlo/internal/url"
)

type Index struct {
	client  *ent.Client
	space   string
	timeout time.Duration
}

var _ relations.Index = (*Index)(nil)

// New borrows the caller's Ent client; the caller owns its lifecycle.
func New(client *ent.Client, space string, timeout time.Duration) (*Index, error) {
	if client == nil || space == "" || timeout < 0 {
		return nil, errors.New("postgres owner index: Ent client, space and valid timeout required")
	}
	return &Index{client: client, space: space, timeout: cmp.Or(timeout, time.Second)}, nil
}

// Check verifies all entity columns without applying migrations or reading data.
func (i *Index) Check(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, i.timeout)
	defer cancel()
	_, err := i.client.OwnerLinkIndex.Query().Limit(0).All(ctx)
	return err
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
	tx, err := i.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// PostgreSQL-specific locking uses Ent's transaction escape hatch. All row
	// operations below use generated builders on that same transaction.
	// Hash collisions only serialize unrelated keys; codes cannot contain '/'.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", i.space+"/"+code); err != nil {
		return err
	}
	r, err := source.Get(ctx, code)
	if err != nil && !errors.Is(err, url.ErrNotFound) {
		return err
	}
	if err == nil && (r == nil || r.Code != code || r.OwnerID != "" && r.CreatedAt.IsZero()) {
		return errors.New("postgres owner index: invalid primary record")
	}
	if errors.Is(err, url.ErrNotFound) || r.OwnerID == "" {
		_, err = tx.OwnerLinkIndex.Delete().Where(
			ownerlinkindex.LinkSpaceEQ(i.space), ownerlinkindex.CodeEQ(code),
		).Exec(ctx)
	} else {
		target := sql.Dialect(dialect.Postgres).Table(ownerlinkindex.Table)
		excluded := sql.Dialect(dialect.Postgres).Table("excluded")
		err = tx.OwnerLinkIndex.Create().
			SetLinkSpace(i.space).SetCode(code).SetOwnerID(r.OwnerID).SetLinkCreatedAt(r.CreatedAt).
			OnConflict(
				sql.ConflictColumns(ownerlinkindex.FieldLinkSpace, ownerlinkindex.FieldCode),
				// Both columns are NOT NULL. Avoid writes for unchanged refs,
				// including nanoseconds lost to PostgreSQL's microsecond precision.
				sql.UpdateWhere(sql.Or(
					sql.ColumnsNEQ(target.C(ownerlinkindex.FieldOwnerID), excluded.C(ownerlinkindex.FieldOwnerID)),
					sql.ColumnsNEQ(target.C(ownerlinkindex.FieldLinkCreatedAt), excluded.C(ownerlinkindex.FieldLinkCreatedAt)),
				)),
			).Update(func(u *ent.OwnerLinkIndexUpsert) {
			u.UpdateOwnerID().UpdateLinkCreatedAt().UpdateIndexedAt()
		}).Exec(ctx)
		// Ent's single-row UPSERT expects RETURNING id. The conditional
		// no-op legitimately returns no row; the transaction remains valid.
		if errors.Is(err, stdsql.ErrNoRows) {
			err = nil
		}
	}
	if err != nil {
		return err
	}
	return tx.Commit()
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
	query := i.client.OwnerLinkIndex.Query().Where(
		ownerlinkindex.LinkSpaceEQ(i.space), ownerlinkindex.OwnerIDEQ(owner),
	)
	if after != nil {
		query.Where(func(s *sql.Selector) {
			s.Where(sql.CompositeLT(s.Columns(ownerlinkindex.FieldLinkCreatedAt, ownerlinkindex.FieldCode), after.CreatedAt, after.Code))
		})
	}
	// Scan the two projection columns only, preserving covering-index reads
	// instead of loading Ent entity IDs that hydration does not need.
	var rows []struct {
		Code      string    `sql:"code"`
		CreatedAt time.Time `sql:"link_created_at"`
	}
	err := query.Order(ownerlinkindex.ByLinkCreatedAt(sql.OrderDesc()), ownerlinkindex.ByCode(sql.OrderDesc())).
		Limit(limit).Select(ownerlinkindex.FieldCode, ownerlinkindex.FieldLinkCreatedAt).Scan(ctx, &rows)
	if err != nil {
		return nil, err
	}
	refs := make([]relations.Ref, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, relations.Ref{Code: row.Code, CreatedAt: row.CreatedAt})
	}
	return refs, nil
}

func (i *Index) ScanCodes(parent context.Context, after string, limit int) ([]string, error) {
	if limit < 1 || limit > 1000 {
		return nil, url.ErrInvalidPage
	}
	ctx, cancel := context.WithTimeout(parent, i.timeout)
	defer cancel()
	return i.client.OwnerLinkIndex.Query().
		Where(ownerlinkindex.LinkSpaceEQ(i.space), ownerlinkindex.CodeGT(after)).
		Order(ownerlinkindex.ByCode()).Limit(limit).Select(ownerlinkindex.FieldCode).Strings(ctx)
}
