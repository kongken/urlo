package schema

import (
	"regexp"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// OwnerLinkIndex is a disposable owner projection, not an authoritative link.
// Its row ID must never be used as the identity of an S3 link generation.
type OwnerLinkIndex struct {
	ent.Schema
}

func (OwnerLinkIndex) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id"),
		field.String("link_space").NotEmpty().Immutable().
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Annotations(entsql.Annotation{Collation: "C"}),
		field.String("code").Match(regexp.MustCompile(`^[A-Za-z0-9]{1,32}$`)).Immutable().
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Annotations(entsql.Annotation{Collation: "C"}),
		field.String("owner_id").NotEmpty().
			SchemaType(map[string]string{dialect.Postgres: "text"}).
			Annotations(entsql.Annotation{Collation: "C"}),
		field.Time("link_created_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("indexed_at").Default(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}).
			Annotations(entsql.DefaultExpr("now()")),
	}
}

func (OwnerLinkIndex) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("link_space", "code").Unique().StorageKey("owner_link_index_space_code_key"),
		index.Fields("link_space", "owner_id", "link_created_at", "code").
			StorageKey("owner_link_index_owner_page_idx").
			Annotations(entsql.DescColumns("link_created_at", "code")),
	}
}

func (OwnerLinkIndex) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{
		Table: "owner_link_index",
		Checks: map[string]string{
			"owner_link_index_link_space_check": "link_space <> ''",
			"owner_link_index_code_check":       "code ~ '^[A-Za-z0-9]{1,32}$'",
			"owner_link_index_owner_id_check":   "owner_id <> ''",
		},
	}}
}
