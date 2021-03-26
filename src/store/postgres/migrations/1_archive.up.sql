-- This migration creates an archive schema and sets up triggers to
-- automatically back up nodes and node_revisions data on delete. This allows
-- the data to be deleted from the original `public.nodes` and
-- `audit.node_revisions` tables, keeping those tables updated with only the
-- non-deleted data, which speeds up read times. The deleted data is instead
-- archived in a separate schema called `archive`. The rationale for this is
-- that deleted nodes and their revisions will be rarely accessed compared to
-- non-deleted nodes. Keeping the logically deleted data in the same tables and
-- using a flag to mark deleted content will bloat the tables and make querying
-- slow as the database will have to go through deleted data every time. A
-- midway arrangement would have been to use indexes with `where` clauses to
-- make sure only non-deleted data is indexed. That would still need us to have
-- complex indexes and complex rules for data partitioning in future. An
-- additional benefit of the current approach is that the `archive` data is on a
-- different schema which can make it easier to split archived and non-archived
-- data when partitioning in the future.

-- Create a different schema for storing archives
create schema archive;

-- Create table archive.nodes to store deleted nodes data
create table archive.nodes
    (like public.nodes excluding constraints);

-- Add primary key for archive.nodes as the only index in that table
alter table archive.nodes
    add primary key (id);

-- Add a column to keep track of time of deletion
alter table archive.nodes
    add column deleted_at timestamp with time zone not null default now();

-- Create table archive.node_revisions to store deleted node revision data
create table archive.node_revisions
    (like audit.node_revisions excluding constraints);

-- Add primary key for archive.nodes as the only index in that table
alter table archive.node_revisions
    add primary key (node_id, created_at);

-- Add a column to keep track of time of deletion of revision
alter table archive.node_revisions
    add column deleted_at timestamp with time zone not null default now();

-- Create trigger function to copy nodes data to archive.nodes
create or replace function trigger_archive_node_on_delete()
    returns trigger
    language plpgsql as $body$
begin
    insert into archive.nodes
        (id, author_id, data_type, source_node_id, created_at, in_reply_to,
        attrs, updated_at, updated_by, subject, body, rich_data)
    values
        (old.id, old.author_id, old.data_type, old.source_node_id, old.created_at, old.in_reply_to,
        old.attrs, old.updated_at, old.updated_by, old.subject, old.body, old.rich_data);
    return old;
end; $body$;

-- Create trigger function to copy audit.node_revisions data to archive.node_revisions
create or replace function trigger_archive_node_revision_on_delete()
    returns trigger
    language plpgsql as $body$
begin
    insert into archive.node_revisions
        (node_id, subject, body, rich_data, created_at, committer_id)
    values
        (old.node_id, old.subject, old.body, old.rich_data, old.created_at, old.committer_id);
    return old;
end; $body$;

-- Create trigger to archive node on delete
create trigger trigger_node_archive
  before delete
  on public.nodes
  for each row
  execute procedure trigger_archive_node_on_delete();

-- Create trigger to archive node_revision on delete
create trigger trigger_node_revision_archive
  before delete
  on audit.node_revisions
  for each row
  execute procedure trigger_archive_node_revision_on_delete();
