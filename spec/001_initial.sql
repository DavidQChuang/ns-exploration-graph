-- Proposed schema, not an executed migration. See SPEC.md for semantic checks.
-- PostgreSQL; UUIDs are supplied by the application (no UUID extension needed).
BEGIN;
CREATE SCHEMA exploration_graph;

CREATE TABLE exploration_graph.graphs (
    graph_id UUID PRIMARY KEY,
    namespace TEXT NOT NULL CHECK (length(namespace) > 0),
    external_ref TEXT,
    description TEXT NOT NULL,
    schema_version INTEGER NOT NULL DEFAULT 1 CHECK (schema_version = 1),
    state TEXT NOT NULL DEFAULT 'active' CHECK (state IN ('active','closed','archived')),
    revision BIGINT NOT NULL DEFAULT 0 CHECK (revision >= 0),
    metadata JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(metadata) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX graphs_namespace ON exploration_graph.graphs(namespace, graph_id);

CREATE TABLE exploration_graph.nodes (
    graph_id UUID NOT NULL REFERENCES exploration_graph.graphs(graph_id),
    node_id UUID NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('origin','goal','work_item','observation','conclusion','hint')),
    current_revision INTEGER NOT NULL CHECK (current_revision >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (graph_id, node_id)
);
CREATE INDEX nodes_kind ON exploration_graph.nodes(graph_id, kind, node_id);
CREATE UNIQUE INDEX nodes_one_origin ON exploration_graph.nodes(graph_id) WHERE kind = 'origin';

CREATE TABLE exploration_graph.node_revisions (
    graph_id UUID NOT NULL,
    node_id UUID NOT NULL,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    state TEXT NOT NULL CHECK (length(state) > 0),
    title TEXT NOT NULL CHECK (length(title) > 0),
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    actor_id TEXT NOT NULL CHECK (length(actor_id) > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (graph_id, node_id, revision),
    FOREIGN KEY (graph_id, node_id) REFERENCES exploration_graph.nodes(graph_id, node_id)
);
-- A node and its first revision are inserted in the same transaction.
ALTER TABLE exploration_graph.nodes ADD CONSTRAINT nodes_current_revision_fk
    FOREIGN KEY (graph_id, node_id, current_revision)
    REFERENCES exploration_graph.node_revisions(graph_id, node_id, revision)
    DEFERRABLE INITIALLY DEFERRED;

CREATE TABLE exploration_graph.edges (
    graph_id UUID NOT NULL,
    edge_id UUID NOT NULL,
    source_id UUID NOT NULL,
    source_revision INTEGER,
    target_id UUID NOT NULL,
    target_revision INTEGER,
    relation TEXT NOT NULL CHECK (relation IN ('spawns','derived_from','yields','supports','depends_on')),
    actor_id TEXT NOT NULL CHECK (length(actor_id) > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (graph_id, edge_id),
    CHECK (source_id <> target_id),
    CHECK ((relation = 'depends_on' AND source_revision IS NULL AND target_revision IS NULL)
        OR (relation <> 'depends_on' AND source_revision >= 1 AND target_revision >= 1
            AND source_revision IS NOT NULL AND target_revision IS NOT NULL)),
    FOREIGN KEY (graph_id, source_id) REFERENCES exploration_graph.nodes(graph_id, node_id),
    FOREIGN KEY (graph_id, target_id) REFERENCES exploration_graph.nodes(graph_id, node_id),
    FOREIGN KEY (graph_id, source_id, source_revision)
        REFERENCES exploration_graph.node_revisions(graph_id, node_id, revision),
    FOREIGN KEY (graph_id, target_id, target_revision)
        REFERENCES exploration_graph.node_revisions(graph_id, node_id, revision)
);
CREATE UNIQUE INDEX edges_provenance_unique ON exploration_graph.edges
    (graph_id, source_id, source_revision, relation, target_id, target_revision)
    WHERE relation <> 'depends_on';
CREATE UNIQUE INDEX edges_dependency_unique ON exploration_graph.edges
    (graph_id, source_id, target_id) WHERE relation = 'depends_on';
CREATE INDEX edges_incoming ON exploration_graph.edges(graph_id, target_id, relation);
CREATE INDEX edges_outgoing ON exploration_graph.edges(graph_id, source_id, relation);

CREATE TABLE exploration_graph.resource_refs (
    graph_id UUID NOT NULL,
    reference_id UUID NOT NULL,
    node_id UUID NOT NULL,
    node_revision INTEGER NOT NULL,
    resource_type TEXT NOT NULL CHECK (length(resource_type) > 0),
    uri TEXT NOT NULL CHECK (length(uri) > 0),
    external_revision TEXT,
    sha256 TEXT CHECK (sha256 ~ '^[0-9a-f]{64}$'),
    actor_id TEXT NOT NULL CHECK (length(actor_id) > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (graph_id, reference_id),
    FOREIGN KEY (graph_id, node_id, node_revision)
        REFERENCES exploration_graph.node_revisions(graph_id, node_id, revision)
);
CREATE INDEX resource_refs_node ON exploration_graph.resource_refs(graph_id, node_id, node_revision);

CREATE TABLE exploration_graph.goal_acceptances (
    graph_id UUID NOT NULL,
    acceptance_id UUID NOT NULL,
    goal_id UUID NOT NULL,
    evaluated_revision INTEGER NOT NULL,
    resulting_revision INTEGER NOT NULL,
    support_edge_ids JSONB NOT NULL CHECK (jsonb_typeof(support_edge_ids) = 'array'
        AND jsonb_array_length(support_edge_ids) > 0),
    reviewed_resource_ids JSONB NOT NULL CHECK (jsonb_typeof(reviewed_resource_ids) = 'array'),
    actor_id TEXT NOT NULL CHECK (length(actor_id) > 0),
    method TEXT NOT NULL CHECK (method IN ('human','validator','agent')),
    rationale TEXT NOT NULL CHECK (length(rationale) > 0),
    state TEXT NOT NULL CHECK (state IN ('current','stale')),
    stale_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (graph_id, acceptance_id),
    CHECK (resulting_revision = evaluated_revision + 1),
    CHECK (state <> 'stale' OR (stale_reason IS NOT NULL AND length(stale_reason) > 0)),
    FOREIGN KEY (graph_id, goal_id, evaluated_revision)
        REFERENCES exploration_graph.node_revisions(graph_id, node_id, revision),
    FOREIGN KEY (graph_id, goal_id, resulting_revision)
        REFERENCES exploration_graph.node_revisions(graph_id, node_id, revision)
);
-- Command validation verifies support-edge existence, type, graph, and goal.
CREATE UNIQUE INDEX goal_acceptance_current ON exploration_graph.goal_acceptances(graph_id, goal_id)
    WHERE state = 'current';

CREATE TABLE exploration_graph.graph_events (
    graph_id UUID NOT NULL REFERENCES exploration_graph.graphs(graph_id),
    revision BIGINT NOT NULL CHECK (revision >= 1),
    request_id TEXT NOT NULL CHECK (length(request_id) > 0),
    request_hash TEXT NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    command_kind TEXT NOT NULL CHECK (length(command_kind) > 0),
    actor_id TEXT NOT NULL CHECK (length(actor_id) > 0),
    data JSONB NOT NULL CHECK (jsonb_typeof(data) = 'object'),
    result JSONB NOT NULL CHECK (jsonb_typeof(result) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (graph_id, revision),
    UNIQUE (graph_id, request_id)
);
-- Semantic commands own immutability, state rules, revision increments,
-- cycle checks, support invalidation, and event insertion under a graph lock.
-- This DDL alone is not the library and is not a database authorization policy.
COMMIT;
