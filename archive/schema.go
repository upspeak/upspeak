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

-- Nodes. Body content is stored as files in the content/ directory,
-- not in SQLite. This supports the local/remote archive split where
-- local stores files on disk and remote stores in object storage.
CREATE TABLE IF NOT EXISTS nodes (
	id           TEXT PRIMARY KEY,
	short_id     TEXT NOT NULL,
	repo_id      TEXT NOT NULL,
	type         TEXT NOT NULL,
	subject      TEXT NOT NULL,
	content_type TEXT NOT NULL,
	metadata     TEXT,
	source_id    TEXT,
	external_id  TEXT,
	created_by   TEXT NOT NULL,
	version      INTEGER NOT NULL DEFAULT 1,
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_nodes_repo_id ON nodes(repo_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_nodes_source_external ON nodes(source_id, external_id) WHERE source_id IS NOT NULL;

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

-- Filters.
CREATE TABLE IF NOT EXISTS filters (
	id          TEXT PRIMARY KEY,
	short_id    TEXT NOT NULL,
	repo_id     TEXT NOT NULL,
	name        TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	mode        TEXT NOT NULL DEFAULT 'all',
	conditions  TEXT NOT NULL DEFAULT '[]',
	created_by  TEXT NOT NULL,
	version     INTEGER NOT NULL DEFAULT 1,
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_filters_repo_id ON filters(repo_id);

-- Jobs.
CREATE TABLE IF NOT EXISTS jobs (
	id           TEXT PRIMARY KEY,
	short_id     TEXT NOT NULL,
	repo_id      TEXT NOT NULL,
	type         TEXT NOT NULL,
	status       TEXT NOT NULL DEFAULT 'pending',
	params       TEXT,
	started_at   TEXT,
	completed_at TEXT,
	result       TEXT,
	error        TEXT,
	created_by   TEXT NOT NULL,
	created_at   TEXT NOT NULL,
	updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_jobs_repo_id ON jobs(repo_id);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);

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

-- Sources.
CREATE TABLE IF NOT EXISTS sources (
	id               TEXT PRIMARY KEY,
	short_id         TEXT NOT NULL,
	repo_id          TEXT NOT NULL,
	connection_id    TEXT,
	name             TEXT NOT NULL,
	connector        TEXT NOT NULL,
	config           TEXT NOT NULL DEFAULT '{}',
	filter_ids       TEXT NOT NULL DEFAULT '[]',
	filter_chain_mode TEXT NOT NULL DEFAULT 'all',
	rate_limit       TEXT,
	status           TEXT NOT NULL DEFAULT 'active',
	version          INTEGER NOT NULL DEFAULT 1,
	created_by       TEXT NOT NULL,
	created_at       TEXT NOT NULL,
	updated_at       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sources_repo_id ON sources(repo_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sources_repo_short_id ON sources(repo_id, short_id);

-- Sinks.
CREATE TABLE IF NOT EXISTS sinks (
	id               TEXT PRIMARY KEY,
	short_id         TEXT NOT NULL,
	repo_id          TEXT NOT NULL,
	connection_id    TEXT,
	name             TEXT NOT NULL,
	connector        TEXT NOT NULL,
	config           TEXT NOT NULL DEFAULT '{}',
	filter_ids       TEXT NOT NULL DEFAULT '[]',
	filter_chain_mode TEXT NOT NULL DEFAULT 'all',
	rate_limit       TEXT,
	status           TEXT NOT NULL DEFAULT 'active',
	version          INTEGER NOT NULL DEFAULT 1,
	created_by       TEXT NOT NULL,
	created_at       TEXT NOT NULL,
	updated_at       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sinks_repo_id ON sinks(repo_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_sinks_repo_short_id ON sinks(repo_id, short_id);

-- Connections: owner-scoped, configure-once links to external systems.
-- Credentials are stored encrypted (BLOB) and never returned in API responses.
CREATE TABLE IF NOT EXISTS connections (
	id                TEXT PRIMARY KEY,
	short_id          TEXT NOT NULL,
	owner_id          TEXT NOT NULL,
	name              TEXT NOT NULL,
	connector         TEXT NOT NULL,
	auth_type         TEXT NOT NULL DEFAULT 'none',
	config            TEXT NOT NULL DEFAULT '{}',
	credentials       BLOB,
	status            TEXT NOT NULL DEFAULT 'active',
	rate_limit        TEXT,
	credential_expiry TEXT,
	last_checked_at   TEXT,
	last_error        TEXT,
	version           INTEGER NOT NULL DEFAULT 1,
	created_by        TEXT NOT NULL,
	created_at        TEXT NOT NULL,
	updated_at        TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_connections_owner ON connections(owner_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_connections_owner_short_id ON connections(owner_id, short_id);

-- Ingest cursors: per-source resumption point (opaque, adapter-defined).
CREATE TABLE IF NOT EXISTS ingest_cursors (
	source_id  TEXT PRIMARY KEY,
	cursor     TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

-- Collection history.
CREATE TABLE IF NOT EXISTS collection_history (
	id            TEXT PRIMARY KEY,
	source_id     TEXT NOT NULL,
	at            TEXT NOT NULL,
	result        TEXT NOT NULL,
	details       TEXT NOT NULL DEFAULT '{}',
	error_message TEXT NOT NULL DEFAULT '',
	duration_ms   INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_collection_history_source ON collection_history(source_id, at DESC);

-- Publish history.
CREATE TABLE IF NOT EXISTS publish_history (
	id            TEXT PRIMARY KEY,
	sink_id       TEXT NOT NULL,
	at            TEXT NOT NULL,
	result        TEXT NOT NULL,
	details       TEXT NOT NULL DEFAULT '{}',
	error_message TEXT NOT NULL DEFAULT '',
	duration_ms   INTEGER NOT NULL DEFAULT 0,
	external_url  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_publish_history_sink ON publish_history(sink_id, at DESC);

-- Schedules.
CREATE TABLE IF NOT EXISTS schedules (
	id         TEXT PRIMARY KEY,
	short_id   TEXT NOT NULL UNIQUE,
	name       TEXT NOT NULL,
	cron       TEXT NOT NULL,
	action     TEXT NOT NULL,
	enabled    INTEGER NOT NULL DEFAULT 1,
	next_run   TEXT,
	last_run   TEXT,
	version    INTEGER NOT NULL DEFAULT 1,
	created_by TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

-- Rules.
CREATE TABLE IF NOT EXISTS rules (
	id         TEXT PRIMARY KEY,
	short_id   TEXT NOT NULL,
	repo_id    TEXT NOT NULL,
	name       TEXT NOT NULL,
	trigger    TEXT NOT NULL DEFAULT '{}',
	actions    TEXT NOT NULL DEFAULT '[]',
	status     TEXT NOT NULL DEFAULT 'active',
	version    INTEGER NOT NULL DEFAULT 1,
	created_by TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_rules_repo_id ON rules(repo_id);
CREATE INDEX IF NOT EXISTS idx_rules_repo_status ON rules(repo_id, status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_rules_short_id ON rules(repo_id, short_id);

-- Rule execution history.
CREATE TABLE IF NOT EXISTS rule_executions (
	id               TEXT PRIMARY KEY,
	rule_id          TEXT NOT NULL,
	event_id         TEXT NOT NULL,
	event_type       TEXT NOT NULL,
	actions_executed TEXT NOT NULL DEFAULT '[]',
	at               TEXT NOT NULL,
	duration_ms      INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_rule_executions_rule ON rule_executions(rule_id, at DESC);

-- Enable WAL mode for better concurrent read performance.
PRAGMA journal_mode=WAL;
`

// ftsSchemaSQL defines the FTS5 virtual table for full-text search.
// This is applied separately from the main schema because FTS5 requires
// the sqlite3 library to be compiled with SQLITE_ENABLE_FTS5.
const ftsSchemaSQL = `
-- Full-text search index for nodes.
CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
	node_id UNINDEXED,
	repo_id UNINDEXED,
	subject,
	body_text
);
`
