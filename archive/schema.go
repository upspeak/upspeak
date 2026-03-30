package archive

// schemaSQL defines the SQLite schema for all entity tables.
const schemaSQL = `
-- Repositories.
CREATE TABLE IF NOT EXISTS repositories (
	id          TEXT PRIMARY KEY,
	short_id    TEXT NOT NULL,
	slug        TEXT NOT NULL,
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	owner_id    TEXT NOT NULL,
	version     INTEGER NOT NULL DEFAULT 1,
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_repositories_owner_slug ON repositories(owner_id, slug);

-- Nodes.
CREATE TABLE IF NOT EXISTS nodes (
	id           TEXT PRIMARY KEY,
	short_id     TEXT NOT NULL,
	repo_id      TEXT NOT NULL,
	type         TEXT NOT NULL,
	subject      TEXT NOT NULL,
	content_type TEXT NOT NULL,
	body         TEXT,
	metadata     TEXT,
	created_by   TEXT NOT NULL,
	version      INTEGER NOT NULL DEFAULT 1,
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_nodes_repo_id ON nodes(repo_id);

-- Edges.
CREATE TABLE IF NOT EXISTS edges (
	id         TEXT PRIMARY KEY,
	short_id   TEXT NOT NULL,
	repo_id    TEXT NOT NULL,
	type       TEXT NOT NULL,
	source     TEXT NOT NULL,
	target     TEXT NOT NULL,
	label      TEXT NOT NULL DEFAULT '',
	weight     REAL NOT NULL DEFAULT 1.0,
	created_by TEXT NOT NULL,
	version    INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_edges_repo_id ON edges(repo_id);
CREATE INDEX IF NOT EXISTS idx_edges_source ON edges(source);
CREATE INDEX IF NOT EXISTS idx_edges_target ON edges(target);

-- Threads.
CREATE TABLE IF NOT EXISTS threads (
	id         TEXT PRIMARY KEY,
	short_id   TEXT NOT NULL,
	repo_id    TEXT NOT NULL,
	node_id    TEXT NOT NULL,
	metadata   TEXT,
	created_by TEXT NOT NULL,
	version    INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	FOREIGN KEY (node_id) REFERENCES nodes(id)
);
CREATE INDEX IF NOT EXISTS idx_threads_repo_id ON threads(repo_id);

-- Thread-edge links.
CREATE TABLE IF NOT EXISTS thread_edges (
	thread_id TEXT NOT NULL,
	edge_id   TEXT NOT NULL,
	PRIMARY KEY (thread_id, edge_id),
	FOREIGN KEY (thread_id) REFERENCES threads(id),
	FOREIGN KEY (edge_id) REFERENCES edges(id)
);

-- Annotations.
CREATE TABLE IF NOT EXISTS annotations (
	id         TEXT PRIMARY KEY,
	short_id   TEXT NOT NULL,
	repo_id    TEXT NOT NULL,
	node_id    TEXT NOT NULL,
	edge_id    TEXT NOT NULL,
	motivation TEXT NOT NULL,
	created_by TEXT NOT NULL,
	version    INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	FOREIGN KEY (node_id) REFERENCES nodes(id),
	FOREIGN KEY (edge_id) REFERENCES edges(id)
);
CREATE INDEX IF NOT EXISTS idx_annotations_repo_id ON annotations(repo_id);

-- Per-repo sequences for entity short IDs.
CREATE TABLE IF NOT EXISTS repo_sequences (
	repo_id  TEXT NOT NULL,
	entity   TEXT NOT NULL,
	next_seq INTEGER NOT NULL DEFAULT 1,
	PRIMARY KEY (repo_id, entity)
);

-- Global sequences (schedule, job, user).
CREATE TABLE IF NOT EXISTS global_sequences (
	entity   TEXT PRIMARY KEY,
	next_seq INTEGER NOT NULL DEFAULT 1
);

-- Per-user sequences (repository short IDs).
CREATE TABLE IF NOT EXISTS user_sequences (
	owner_id TEXT NOT NULL,
	entity   TEXT NOT NULL,
	next_seq INTEGER NOT NULL DEFAULT 1,
	PRIMARY KEY (owner_id, entity)
);

-- Repo slug redirects for renamed repositories.
CREATE TABLE IF NOT EXISTS repo_slug_redirects (
	old_slug   TEXT NOT NULL,
	owner_id   TEXT NOT NULL,
	repo_id    TEXT NOT NULL,
	created_at TEXT NOT NULL,
	PRIMARY KEY (old_slug, owner_id)
);

-- Enable WAL mode for better concurrent read performance.
PRAGMA journal_mode=WAL;
`
