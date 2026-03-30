# User Journeys

## Journey 1: Researcher collecting information

Person A is researching AI governance. They have Upspeak collecting from RSS feeds, Hacker News, and a Discourse forum. They read on their phone during their commute and annotate on their laptop later.

1. **Configure sources** — Person A adds an RSS feed URL and a Discourse instance as sources, sets collection rules (keywords, frequency) using filters
2. **System collects in background** — Upspeak fetches on schedule, applies source filters, deduplicates, creates nodes, links related content via edges
3. **Browse what's new** — Person A opens their phone, sees a feed of recently collected items, filtered by their repo
4. **Read and annotate** — They read a node, highlight a passage, add a comment (annotation with motivation)
5. **Thread a conversation** — They link three related articles into a thread with their own commentary node
6. **Search across knowledge** — "Show me everything about EU AI Act from the last month"
7. **Share selectively** — They publish a curated thread to their Fediverse followers via a sink

## Journey 2: Developer using Upspeak as a knowledge backend

Person B is building a documentation tool. They use Upspeak's API to store and retrieve structured content. Their app is one of several clients.

1. **Create a repo** — Person B creates a dedicated repo for their app's content
2. **Store structured content** — POST nodes with specific types and content types
3. **Build relationships** — Create edges between nodes (doc -> section -> subsection)
4. **Query by structure** — "Give me all nodes of type 'section' connected to this parent doc node" via graph traversal
5. **Subscribe to changes** — WebSocket connection to get notified when content changes
6. **Bulk operations** — Import a batch of nodes and edges from a migration

## Journey 3: AI agent processing content

An AI agent (e.g., Claude) is configured to summarise, categorise, and relate incoming content in the background.

1. **Subscribe to new content** — Agent listens for new nodes via WebSocket or triggered by a rule (webhook)
2. **Read and process** — Fetches the node, generates a summary, extracts entities
3. **Enrich the graph** — Creates annotation nodes (summary, entities), links them via edges
4. **Classify and tag** — Updates node metadata with categories, sentiment via PATCH
5. **Scheduled batch work** — Every 6 hours, re-classify all untagged nodes via scheduled webhook
6. **Report** — When the user connects, they see enriched, categorised content

## Journey 4: Multi-device, offline user

Person C uses Upspeak on their laptop at home (self-hosted), work machine, and phone. They're sometimes offline on flights.

1. **Work offline** — Creates nodes and annotations locally while on a flight — writes succeed immediately (synchronous to local archive)
2. **Reconnect and sync** — On landing, Upspeak syncs local changes to home server via JetStream
3. **See background updates** — Home server has been collecting RSS and processing content while they were away — they see new items
4. **Resolve conflicts** — If they edited the same node on two devices, Upspeak shows the conflict and offers resolution options

## Journey 5: Social knowledge sharing

Person A and Person D are both Upspeak users. Person A wants to share a curated collection about climate policy with Person D.

1. **Curate a collection** — Person A creates a thread of related nodes
2. **Publish to network** — Person A publishes the thread with visibility settings
3. **Discover and collect** — Person D sees Person A's shared thread, pulls it into their own repo (follow mode creates a source)
4. **Annotate independently** — Person D adds their own annotations — these stay in their repo
5. **Follow updates** — If Person A updates the thread, Person D sees the changes via the source subscription

## Journey 6: Publisher

Person E runs a newsletter. They configure Upspeak to collect from their annotated threads and publish a weekly digest.

1. **Create a digest thread** — Person E creates a thread for this week's digest
2. **Set up automation** — A rule adds any annotation with motivation "newsletter" to the digest thread
3. **Register sinks** — RSS feed generator and email service webhook
4. **Schedule weekly publish** — Cron jobs trigger publish to both sinks every Friday at 9am
5. **Tag items during the week** — Each annotation with motivation "newsletter" auto-adds to the digest
6. **Preview and publish** — Dry-run the sink before the scheduled publish; manual trigger if needed

## Journey 7: Repo pipeline

Person A collects raw content from many sources into a "Raw" repo. They want a "Curated" repo with only AI-related content, and a "Newsletter" repo with only high-priority curated items.

1. **Create pipeline repos** — Raw Collection, Curated AI, Newsletter
2. **Raw collects from external sources** — RSS, Discourse, etc.
3. **Curated subscribes to Raw** — Using `repo` connector with a filter (only AI articles)
4. **Newsletter subscribes to Curated** — Using `repo` connector with a filter (only high-priority)
5. **Pipeline flows automatically** — RSS -> Raw -> Curated -> Newsletter -> Sinks
