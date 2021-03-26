-- Drop nodes archive table
drop table archive.nodes cascade;

-- Drop node revisions archive table
drop table archive.node_revisions cascade;

-- Drop trigger function for node archive on delete
drop trigger trigger_node_archive on public.nodes;

-- Drop trigger function for node archive on delete
drop trigger trigger_node_revision_archive on audit.node_revisions;

-- Drop node archive function
drop function trigger_archive_node_on_delete() cascade;

-- Drop node revisionss archive function
drop function trigger_archive_node_revision_on_delete() cascade;

-- Drop the archive schema
drop schema archive;
