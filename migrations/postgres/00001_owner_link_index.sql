-- +goose Up
CREATE TABLE owner_link_index (
    link_space      text COLLATE "C" NOT NULL CHECK (link_space <> ''),
    code            text COLLATE "C" NOT NULL CHECK (code ~ '^[A-Za-z0-9]{1,32}$'),
    owner_id        text COLLATE "C" NOT NULL CHECK (owner_id <> ''),
    link_created_at timestamptz NOT NULL,
    indexed_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (link_space, code)
);

CREATE INDEX owner_link_index_owner_page_idx
    ON owner_link_index (link_space, owner_id, link_created_at DESC, code DESC);

-- +goose Down
DROP TABLE owner_link_index;
