// Package postgres implements normalized, graph-row-serialized PostgreSQL storage.
package postgres

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"ns-exploration-graph/graph"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/001_initial.sql
var migration string

type Store struct {
	db    *sql.DB
	fault func(string) error
}

// WithFaultInjector is intended for deterministic transaction rollback tests.
func WithFaultInjector(fn func(string) error) func(*Store) { return func(s *Store) { s.fault = fn } }
func Open(ctx context.Context, dsn string, options ...func(*Store)) (*Store, error) {
	db, e := sql.Open("pgx", dsn)
	if e != nil {
		return nil, e
	}
	s := &Store{db: db}
	for _, option := range options {
		option(s)
	}
	if e = db.PingContext(ctx); e != nil {
		db.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) DB() *sql.DB  { return s.db }
func (s *Store) Migrate(ctx context.Context) error {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if _, e = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(587324921)`); e != nil {
		return e
	}
	var exists bool
	if e = tx.QueryRowContext(ctx, `SELECT to_regclass('exploration_graph.graphs') IS NOT NULL`).Scan(&exists); e != nil {
		return e
	}
	if !exists {
		ddl := strings.ReplaceAll(strings.ReplaceAll(migration, "BEGIN;", ""), "COMMIT;", "")
		if _, e = tx.ExecContext(ctx, ddl); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func storageError(e error) error {
	if e == nil {
		return nil
	}
	var ge *graph.Error
	if errors.As(e, &ge) {
		return e
	}
	// SPEC: error text never includes SQL or credentials. Driver detail stays out of the returned error.
	return graph.Err("storage_unavailable", "Graph service is unavailable.")
}
func (s *Store) checkpoint(name string) error {
	if s.fault != nil {
		return s.fault(name)
	}
	return nil
}
func (s *Store) Update(ctx context.Context, ns, id string, create *graph.Graph, fn func(*graph.State) error) error {
	return s.update(ctx, ns, id, create, nil, fn)
}

// UpdateRequest loads only the committed event for requestID through the
// (graph_id, request_id) unique index instead of the complete event history.
func (s *Store) UpdateRequest(ctx context.Context, ns, id string, create *graph.Graph, requestID string, fn func(*graph.State) error) error {
	return s.update(ctx, ns, id, create, &requestID, fn)
}
func (s *Store) update(ctx context.Context, ns, id string, create *graph.Graph, requestID *string, fn func(*graph.State) error) (err error) {
	defer func() { err = storageError(err) }()
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if create != nil {
		metadata, e := json.Marshal(create.Metadata)
		if e != nil {
			return e
		}
		if string(metadata) == "null" {
			metadata = []byte("{}")
		}
		_, e = tx.ExecContext(ctx, `INSERT INTO exploration_graph.graphs(graph_id,namespace,external_ref,description,state,revision,metadata,created_at,updated_at) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9) ON CONFLICT(graph_id) DO NOTHING`, id, ns, create.ExternalRef, create.Description, create.State, create.Revision, metadata, create.CreatedAt, create.UpdatedAt)
		if e != nil {
			return e
		}
	}
	var actual string
	e = tx.QueryRowContext(ctx, `SELECT namespace FROM exploration_graph.graphs WHERE graph_id=$1 FOR UPDATE`, id).Scan(&actual)
	if e == sql.ErrNoRows {
		return graph.Err("not_found", "graph not found")
	}
	if e != nil {
		return e
	}
	if actual != ns {
		return graph.Err("permission_denied", "Namespace does not match.")
	}
	state, e := load(ctx, tx, id, requestID == nil)
	if e != nil {
		return e
	}
	if requestID != nil {
		if state.Events, e = readRequestEvent(ctx, tx, id, *requestID); e != nil {
			return e
		}
	}
	before := newBaseline(state)
	if e = fn(state); e != nil {
		return e
	}
	if state.Graph.ID != id || state.Graph.Namespace != ns {
		return graph.Err("invalid_argument", "graph identity cannot change")
	}
	if e = s.persist(ctx, tx, before, state); e != nil {
		return e
	}
	if e = s.checkpoint("before_commit"); e != nil {
		return e
	}
	return tx.Commit()
}
func (s *Store) Read(ctx context.Context, ns, id string) (state *graph.State, err error) {
	defer func() { err = storageError(err) }()
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	state, e = load(ctx, tx, id, false)
	if e != nil {
		return nil, e
	}
	if state.Graph.Namespace != ns {
		return nil, graph.Err("permission_denied", "Namespace does not match.")
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return state, nil
}

// ReadScoped reads in one repeatable-read transaction: the graph row, current node
// revisions through the current-revision key, all edges, and for a detail node its
// history, resource references and acceptances through graph-local indexes.
func (s *Store) ReadScoped(ctx context.Context, ns, id string, scope graph.ReadScope) (state *graph.State, err error) {
	defer func() { err = storageError(err) }()
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	st, e := loadGraph(ctx, tx, id)
	if e != nil {
		return nil, e
	}
	if st.Graph.Namespace != ns {
		return nil, graph.Err("permission_denied", "Namespace does not match.")
	}
	indexes, e := loadNodes(ctx, tx, st, true, `SELECT n.node_id::text,n.kind,n.current_revision,n.created_at,r.revision,r.state,r.title,r.payload,r.actor_id,r.created_at FROM exploration_graph.nodes n JOIN exploration_graph.node_revisions r ON r.graph_id=n.graph_id AND r.node_id=n.node_id AND r.revision=n.current_revision WHERE n.graph_id=$1 ORDER BY n.node_id`, id)
	if e != nil {
		return nil, e
	}
	if e = loadEdges(ctx, tx, st, id); e != nil {
		return nil, e
	}
	if i, ok := indexes[scope.DetailNodeID]; ok && graph.ValidID(scope.DetailNodeID) {
		st.Nodes[i].Revisions = []graph.Revision{}
		if e = loadRevisions(ctx, tx, st, indexes, `SELECT node_id::text,revision,state,title,payload,actor_id,created_at FROM exploration_graph.node_revisions WHERE graph_id=$1 AND node_id=$2 ORDER BY revision`, id, scope.DetailNodeID); e != nil {
			return nil, e
		}
		if e = loadResourceRefs(ctx, tx, st, ` AND node_id=$2`, id, scope.DetailNodeID); e != nil {
			return nil, e
		}
		if e = loadAcceptances(ctx, tx, st, ` AND goal_id=$2`, id, scope.DetailNodeID); e != nil {
			return nil, e
		}
	}
	st.Events = nil
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return st, nil
}
func load(ctx context.Context, tx *sql.Tx, id string, events bool) (*graph.State, error) {
	st, e := loadGraph(ctx, tx, id)
	if e != nil {
		return nil, e
	}
	indexes, e := loadNodes(ctx, tx, st, false, `SELECT node_id::text,kind,current_revision,created_at FROM exploration_graph.nodes WHERE graph_id=$1 ORDER BY node_id`, id)
	if e != nil {
		return nil, e
	}
	if e = loadRevisions(ctx, tx, st, indexes, `SELECT node_id::text,revision,state,title,payload,actor_id,created_at FROM exploration_graph.node_revisions WHERE graph_id=$1 ORDER BY node_id,revision`, id); e != nil {
		return nil, e
	}
	if e = loadEdges(ctx, tx, st, id); e != nil {
		return nil, e
	}
	if e = loadResourceRefs(ctx, tx, st, ``, id); e != nil {
		return nil, e
	}
	if e = loadAcceptances(ctx, tx, st, ``, id); e != nil {
		return nil, e
	}
	if events {
		st.Events, e = readEvents(ctx, tx, id, 0, 0)
		if e != nil {
			return nil, e
		}
	}
	return st, nil
}
func loadGraph(ctx context.Context, tx *sql.Tx, id string) (*graph.State, error) {
	st := graph.EmptyState(graph.Graph{})
	var metadata []byte
	e := tx.QueryRowContext(ctx, `SELECT graph_id::text,namespace,COALESCE(external_ref,''),description,state,revision,metadata,created_at,updated_at FROM exploration_graph.graphs WHERE graph_id=$1`, id).Scan(&st.Graph.ID, &st.Graph.Namespace, &st.Graph.ExternalRef, &st.Graph.Description, &st.Graph.State, &st.Graph.Revision, &metadata, &st.Graph.CreatedAt, &st.Graph.UpdatedAt)
	if e == sql.ErrNoRows {
		return nil, graph.Err("not_found", "graph not found")
	}
	if e != nil {
		return nil, e
	}
	if e = json.Unmarshal(metadata, &st.Graph.Metadata); e != nil {
		return nil, e
	}
	return st, nil
}

// loadNodes scans node rows; with current set, each row also carries the current revision.
func loadNodes(ctx context.Context, tx *sql.Tx, st *graph.State, current bool, query, id string) (map[string]int, error) {
	rows, e := tx.QueryContext(ctx, query, id)
	if e != nil {
		return nil, e
	}
	indexes := map[string]int{}
	for rows.Next() {
		var n graph.Node
		targets := []any{&n.ID, &n.Kind, &n.CurrentRevision, &n.CreatedAt}
		var r graph.Revision
		if current {
			targets = append(targets, &r.Revision, &r.State, &r.Title, &r.Payload, &r.ActorID, &r.CreatedAt)
		}
		if e = rows.Scan(targets...); e != nil {
			rows.Close()
			return nil, e
		}
		n.Revisions = []graph.Revision{}
		if current {
			n.Revisions = append(n.Revisions, r)
		}
		indexes[n.ID] = len(st.Nodes)
		st.Nodes = append(st.Nodes, n)
	}
	return indexes, closeRows(rows)
}
func loadRevisions(ctx context.Context, tx *sql.Tx, st *graph.State, indexes map[string]int, query string, args ...any) error {
	rows, e := tx.QueryContext(ctx, query, args...)
	if e != nil {
		return e
	}
	for rows.Next() {
		var nid string
		var r graph.Revision
		if e = rows.Scan(&nid, &r.Revision, &r.State, &r.Title, &r.Payload, &r.ActorID, &r.CreatedAt); e != nil {
			rows.Close()
			return e
		}
		i := indexes[nid]
		st.Nodes[i].Revisions = append(st.Nodes[i].Revisions, r)
	}
	return closeRows(rows)
}
func loadEdges(ctx context.Context, tx *sql.Tx, st *graph.State, id string) error {
	rows, e := tx.QueryContext(ctx, `SELECT edge_id::text,source_id::text,COALESCE(source_revision,0),target_id::text,COALESCE(target_revision,0),relation,actor_id,created_at FROM exploration_graph.edges WHERE graph_id=$1 ORDER BY edge_id`, id)
	if e != nil {
		return e
	}
	for rows.Next() {
		var v graph.Edge
		if e = rows.Scan(&v.ID, &v.SourceID, &v.SourceRevision, &v.TargetID, &v.TargetRevision, &v.Relation, &v.ActorID, &v.CreatedAt); e != nil {
			rows.Close()
			return e
		}
		st.Edges = append(st.Edges, v)
	}
	return closeRows(rows)
}
func loadResourceRefs(ctx context.Context, tx *sql.Tx, st *graph.State, filter string, args ...any) error {
	rows, e := tx.QueryContext(ctx, `SELECT reference_id::text,node_id::text,node_revision,resource_type,uri,COALESCE(external_revision,''),COALESCE(sha256,''),actor_id,created_at FROM exploration_graph.resource_refs WHERE graph_id=$1`+filter+` ORDER BY reference_id`, args...)
	if e != nil {
		return e
	}
	for rows.Next() {
		var v graph.ResourceRef
		if e = rows.Scan(&v.ID, &v.NodeID, &v.NodeRevision, &v.ResourceType, &v.URI, &v.ExternalRevision, &v.SHA256, &v.ActorID, &v.CreatedAt); e != nil {
			rows.Close()
			return e
		}
		st.ResourceRefs = append(st.ResourceRefs, v)
	}
	return closeRows(rows)
}
func loadAcceptances(ctx context.Context, tx *sql.Tx, st *graph.State, filter string, args ...any) error {
	rows, e := tx.QueryContext(ctx, `SELECT acceptance_id::text,goal_id::text,evaluated_revision,resulting_revision,support_edge_ids,reviewed_resource_ids,actor_id,method,rationale,state,COALESCE(stale_reason,''),created_at FROM exploration_graph.goal_acceptances WHERE graph_id=$1`+filter+` ORDER BY acceptance_id`, args...)
	if e != nil {
		return e
	}
	for rows.Next() {
		var v graph.Acceptance
		var edges, refs []byte
		if e = rows.Scan(&v.ID, &v.GoalID, &v.EvaluatedRevision, &v.ResultingRevision, &edges, &refs, &v.ActorID, &v.Method, &v.Rationale, &v.State, &v.StaleReason, &v.CreatedAt); e != nil {
			rows.Close()
			return e
		}
		if e = json.Unmarshal(edges, &v.SupportEdgeIDs); e != nil {
			rows.Close()
			return e
		}
		if e = json.Unmarshal(refs, &v.ReviewedResourceIDs); e != nil {
			rows.Close()
			return e
		}
		st.Acceptances = append(st.Acceptances, v)
	}
	return closeRows(rows)
}
func closeRows(rows *sql.Rows) error {
	e := rows.Err()
	c := rows.Close()
	if e != nil {
		return e
	}
	return c
}
func readRequestEvent(ctx context.Context, tx *sql.Tx, id, requestID string) ([]graph.Event, error) {
	return scanEvents(tx.QueryContext(ctx, `SELECT revision,request_id,request_hash,command_kind,actor_id,data,result,created_at FROM exploration_graph.graph_events WHERE graph_id=$1 AND request_id=$2`, id, requestID))
}
func readEvents(ctx context.Context, tx *sql.Tx, id string, after int64, limit int) ([]graph.Event, error) {
	query := `SELECT revision,request_id,request_hash,command_kind,actor_id,data,result,created_at FROM exploration_graph.graph_events WHERE graph_id=$1 AND revision>$2 ORDER BY revision`
	args := []any{id, after}
	if limit > 0 {
		query += ` LIMIT $3`
		args = append(args, limit)
	}
	return scanEvents(tx.QueryContext(ctx, query, args...))
}
func scanEvents(rows *sql.Rows, e error) ([]graph.Event, error) {
	if e != nil {
		return nil, e
	}
	out := []graph.Event{}
	for rows.Next() {
		var v graph.Event
		var result []byte
		if e = rows.Scan(&v.Revision, &v.RequestID, &v.RequestHash, &v.CommandKind, &v.ActorID, &v.Data, &result, &v.CreatedAt); e != nil {
			rows.Close()
			return nil, e
		}
		if e = json.Unmarshal(result, &v.Result); e != nil {
			rows.Close()
			return nil, e
		}
		out = append(out, v)
	}
	return out, closeRows(rows)
}
func (s *Store) EventsAfter(ctx context.Context, ns, id string, after int64, limit int) (events []graph.Event, revision int64, err error) {
	defer func() { err = storageError(err) }()
	tx, e := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if e != nil {
		return nil, 0, e
	}
	defer tx.Rollback()
	var actual string
	e = tx.QueryRowContext(ctx, `SELECT namespace,revision FROM exploration_graph.graphs WHERE graph_id=$1`, id).Scan(&actual, &revision)
	if e == sql.ErrNoRows {
		return nil, 0, graph.Err("not_found", "graph not found")
	}
	if e != nil {
		return nil, 0, e
	}
	if actual != ns {
		return nil, 0, graph.Err("permission_denied", "Namespace does not match.")
	}
	if limit < 1 || limit > 1000 {
		return nil, 0, graph.Err("invalid_argument", "event limit must be 1..1000")
	}
	events, e = readEvents(ctx, tx, id, after, limit)
	if e != nil {
		return nil, 0, e
	}
	return events, revision, tx.Commit()
}

// baseline records the loaded identities persist compares against, without copying
// revision payloads or the rest of the aggregate.
type baseline struct {
	revision    int64
	nodes       map[string]graph.Node
	edges       map[string]graph.Edge
	refs        map[string]bool
	acceptances map[string]graph.Acceptance
}

func newBaseline(st *graph.State) *baseline {
	b := &baseline{revision: st.Graph.Revision, nodes: make(map[string]graph.Node, len(st.Nodes)), edges: make(map[string]graph.Edge, len(st.Edges)), refs: make(map[string]bool, len(st.ResourceRefs)), acceptances: make(map[string]graph.Acceptance, len(st.Acceptances))}
	for _, n := range st.Nodes {
		b.nodes[n.ID] = graph.Node{ID: n.ID, Kind: n.Kind, CurrentRevision: n.CurrentRevision}
	}
	for _, v := range st.Edges {
		b.edges[v.ID] = v
	}
	for _, v := range st.ResourceRefs {
		b.refs[v.ID] = true
	}
	for _, v := range st.Acceptances {
		b.acceptances[v.ID] = graph.Acceptance{ID: v.ID, State: v.State, StaleReason: v.StaleReason}
	}
	return b
}
func (s *Store) persist(ctx context.Context, tx *sql.Tx, before *baseline, after *graph.State) error {
	id := after.Graph.ID
	oldNodes := before.nodes
	for _, n := range after.Nodes {
		old, exists := oldNodes[n.ID]
		if !exists {
			if _, e := tx.ExecContext(ctx, `INSERT INTO exploration_graph.nodes(graph_id,node_id,kind,current_revision,created_at) VALUES($1,$2,$3,$4,$5)`, id, n.ID, n.Kind, n.CurrentRevision, n.CreatedAt); e != nil {
				return e
			}
		} else if n.Kind != old.Kind {
			return fmt.Errorf("node kind is immutable")
		}
		for _, r := range n.Revisions {
			if exists && r.Revision <= old.CurrentRevision {
				continue
			}
			if _, e := tx.ExecContext(ctx, `INSERT INTO exploration_graph.node_revisions(graph_id,node_id,revision,state,title,payload,actor_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, id, n.ID, r.Revision, r.State, r.Title, []byte(r.Payload), r.ActorID, r.CreatedAt); e != nil {
				return e
			}
		}
		if exists && old.CurrentRevision != n.CurrentRevision {
			if _, e := tx.ExecContext(ctx, `UPDATE exploration_graph.nodes SET current_revision=$3 WHERE graph_id=$1 AND node_id=$2`, id, n.ID, n.CurrentRevision); e != nil {
				return e
			}
		}
	}
	if e := s.checkpoint("after_nodes"); e != nil {
		return e
	}
	oldEdges := before.edges
	newEdges := map[string]bool{}
	for _, v := range after.Edges {
		newEdges[v.ID] = true
		if _, ok := oldEdges[v.ID]; ok {
			continue
		}
		var source, target any
		if v.Relation != "depends_on" {
			source = v.SourceRevision
			target = v.TargetRevision
		}
		if _, e := tx.ExecContext(ctx, `INSERT INTO exploration_graph.edges(graph_id,edge_id,source_id,source_revision,target_id,target_revision,relation,actor_id,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, v.ID, v.SourceID, source, v.TargetID, target, v.Relation, v.ActorID, v.CreatedAt); e != nil {
			return e
		}
	}
	for key, v := range oldEdges {
		if !newEdges[key] {
			if v.Relation != "depends_on" {
				return fmt.Errorf("provenance edges are immutable")
			}
			if _, e := tx.ExecContext(ctx, `DELETE FROM exploration_graph.edges WHERE graph_id=$1 AND edge_id=$2`, id, key); e != nil {
				return e
			}
		}
	}
	if e := s.checkpoint("after_edges"); e != nil {
		return e
	}
	oldRefs := before.refs
	for _, v := range after.ResourceRefs {
		if oldRefs[v.ID] {
			continue
		}
		if _, e := tx.ExecContext(ctx, `INSERT INTO exploration_graph.resource_refs(graph_id,reference_id,node_id,node_revision,resource_type,uri,external_revision,sha256,actor_id,created_at) VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,''),NULLIF($8,''),$9,$10)`, id, v.ID, v.NodeID, v.NodeRevision, v.ResourceType, v.URI, v.ExternalRevision, v.SHA256, v.ActorID, v.CreatedAt); e != nil {
			return e
		}
	}
	oldAccept := before.acceptances
	for _, v := range after.Acceptances {
		if old, ok := oldAccept[v.ID]; ok {
			if old.State != v.State || old.StaleReason != v.StaleReason {
				if _, e := tx.ExecContext(ctx, `UPDATE exploration_graph.goal_acceptances SET state=$3,stale_reason=NULLIF($4,'') WHERE graph_id=$1 AND acceptance_id=$2`, id, v.ID, v.State, v.StaleReason); e != nil {
					return e
				}
			}
		}
	}
	for _, v := range after.Acceptances {
		if _, ok := oldAccept[v.ID]; ok {
			continue
		}
		edges, e := json.Marshal(v.SupportEdgeIDs)
		if e != nil {
			return e
		}
		refs, e := json.Marshal(v.ReviewedResourceIDs)
		if e != nil {
			return e
		}
		if string(refs) == "null" {
			refs = []byte("[]")
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO exploration_graph.goal_acceptances(graph_id,acceptance_id,goal_id,evaluated_revision,resulting_revision,support_edge_ids,reviewed_resource_ids,actor_id,method,rationale,state,stale_reason,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,''),$13)`, id, v.ID, v.GoalID, v.EvaluatedRevision, v.ResultingRevision, edges, refs, v.ActorID, v.Method, v.Rationale, v.State, v.StaleReason, v.CreatedAt); e != nil {
			return e
		}
	}
	metadata, e := json.Marshal(after.Graph.Metadata)
	if e != nil {
		return e
	}
	if string(metadata) == "null" {
		metadata = []byte("{}")
	}
	if _, e = tx.ExecContext(ctx, `UPDATE exploration_graph.graphs SET external_ref=NULLIF($2,''),description=$3,state=$4,revision=$5,metadata=$6,updated_at=$7 WHERE graph_id=$1`, id, after.Graph.ExternalRef, after.Graph.Description, after.Graph.State, after.Graph.Revision, metadata, after.Graph.UpdatedAt); e != nil {
		return e
	}
	if e = s.checkpoint("before_events"); e != nil {
		return e
	}
	for _, v := range after.Events {
		if v.Revision <= before.revision {
			continue
		}
		result, e := json.Marshal(v.Result)
		if e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `INSERT INTO exploration_graph.graph_events(graph_id,revision,request_id,request_hash,command_kind,actor_id,data,result,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id, v.Revision, v.RequestID, v.RequestHash, v.CommandKind, v.ActorID, []byte(v.Data), result, v.CreatedAt); e != nil {
			return e
		}
	}
	return s.checkpoint("after_events")
}
